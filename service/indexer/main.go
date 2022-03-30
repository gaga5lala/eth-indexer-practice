package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"eth-indexer-practice/pkg/store"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	logger "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	maxSyncWindow   = 20
	minConfirmation = 12 // https://ethereum.stackexchange.com/a/3009
	// rpc = "https://rpc.ankr.com/eth"
	// rpc = "https://data-seed-prebsc-2-s3.binance.org:8545"
	rpc = "https://data-seed-prebsc-2-s2.binance.org:8545"
)

type Block = store.Block
type Transaction = store.Transaction
type BlockTransaction = store.BlockTransaction
type Logs = store.Logs
type Log = store.Log

func main() {
	// logger.SetLevel(logger.DebugLevel)
	logger.WithFields(logger.Fields{
		"rpc":              rpc,
		"min confirmation": minConfirmation,
	}).Info("Start Indexer")

	ctx := context.Background()
	listenCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	db, err := store.NewPostgres()
	if err != nil {
		panic("failed to connect database")
	}

	err = db.AutoMigrate(&Block{})
	if err != nil {
		panic("failed to migration block table")
	}

	err = db.AutoMigrate(&Transaction{})
	if err != nil {
		panic("failed to migration Transaction table")
	}

	ethclient := newEthclient(rpc)
	defer ethclient.Close()

	for {
		lastOnChainBlockNum, err := ethclient.BlockNumber(ctx)
		if err != nil {
			panic(fmt.Sprintf("fail to get latest block from rpc. %s", err))
		}

		// prevent from uncle block
		lastSafeOnChainBlockNum := lastOnChainBlockNum - minConfirmation
		logger.Info(fmt.Sprintf("Latest block number %d", lastSafeOnChainBlockNum))

		var lastDbBlockNum uint64
		var block Block
		err = db.Last(&block).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// lastDbBlockNum = 0
				lastDbBlockNum = 18002000
			} else {
				panic(fmt.Sprintf("failied to get database block status. %s", err))
			}
		} else {
			lastDbBlockNum = block.BlockNum
		}

		logger.WithFields(logger.Fields{
			"lastest db block":      lastDbBlockNum,
			"lastest onchain block": lastOnChainBlockNum,
			"lastest safe block":    lastSafeOnChainBlockNum,
		}).Info("Block status")

		if lastDbBlockNum == lastSafeOnChainBlockNum {
			logger.Info(fmt.Sprintf("waiting for new block"))
			time.Sleep(5 * time.Second)
			continue
		}

		var targetBlockNum uint64
		if lastSafeOnChainBlockNum-uint64(lastDbBlockNum) >= maxSyncWindow {
			targetBlockNum = uint64(lastDbBlockNum + maxSyncWindow)
		} else {
			targetBlockNum = lastSafeOnChainBlockNum
		}

		logger.Debug(fmt.Sprintf("Get blocks from %d to %d", lastDbBlockNum, targetBlockNum))
		var wg sync.WaitGroup
		for i := lastDbBlockNum + 1; i <= targetBlockNum; i++ {
			go saveBlockAndTransaction(listenCtx, int64(i), &wg)
			wg.Add(1)
		}
		wg.Wait()
	}
}

func saveBlockAndTransaction(ctx context.Context, blockNum int64, wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info(fmt.Sprintf("saveBlockAndTransaction: %d", blockNum))
	db, err := store.NewPostgres()
	if err != nil {
		panic("failed to connect database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get generic database")
	}
	defer sqlDB.Close()

	ethclient := newEthclient(rpc)
	defer ethclient.Close()
	blockData, err := ethclient.BlockByNumber(ctx, big.NewInt(blockNum))
	if err != nil {
		panic(fmt.Sprintf("failed to get block %d. %s", blockNum, err))
	}
	logger.Info(fmt.Sprintf("processing block %d", uint64(blockData.Number().Int64())))
	block := &Block{
		BlockNum:   uint64(blockData.Number().Int64()),
		BlockHash:  blockData.Hash().String(),
		BlockTime:  blockData.Time(),
		ParentHash: blockData.ParentHash().String(),
	}
	transactionsData := blockData.Transactions()
	bt := store.BlockTransaction{}
	for _, tx := range blockData.Transactions() {
		bt = append(bt, tx.Hash().String())
	}
	block.Transactions = &bt
	err = db.Create(block).Error
	if err != nil {
		logger.Error("failed to save block. %s", err)
	}
	// PERF: parallel
	for _, tx := range transactionsData {
		logger.Info(fmt.Sprintf("processing tx %s from block %s", tx.Hash(), blockData.Number()))
		receipt, err := ethclient.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			logger.Error(fmt.Sprintf("failed to get receipt of tx %s. %s", tx.Hash(), err))
			continue
		}

		logs := Logs{}
		for _, log := range receipt.Logs {
			logs = append(logs, &Log{
				Index: log.Index,
				Data:  common.Bytes2Hex(log.Data),
			})
		}

		chainID, err := ethclient.ChainID(ctx)
		if err != nil {
			panic(err)
		}

		// https://github.com/ethereum/go-ethereum/issues/23890
		msg, err := tx.AsMessage(types.LatestSignerForChainID(chainID), nil)

		// FIXME:
		// invalid transaction v, r, s values
		// tx 0xebb2d381942b9f34fc53e4737207d8d5cd1f1c686b7b6963c3947be3f361df5c on ETH mainnet
		if err != nil {
			logger.Error(fmt.Sprintf("failed to perform AsMessage on tx %s", tx.Hash()))
		}

		toString := ""
		if msg.To() != nil {
			toString = msg.To().String()
		}
		transaction := &Transaction{
			TxHash:    tx.Hash().String(),
			To:        toString,
			Data:      hexutil.Encode(tx.Data()),
			BlockHash: blockData.Hash().String(),
			BlockNum:  uint64(blockData.Number().Int64()),
			Logs:      &logs,
		}

		transaction.From = msg.From().String()
		transaction.Value = msg.Value().Int64()
		err = db.Create(transaction).Error
		if err != nil {
			logger.Error("failed to save transaction. %s", err)
		}
	}
	logger.Info(fmt.Sprintf("finish processing txs in block %s", blockData.Number()))
}

// PERF: listen ws for new block event
func newEthclient(rpc string) *ethclient.Client {
	client, err := ethclient.Dial(rpc)
	if err != nil {
		panic(fmt.Sprintf("failed to get ethclient. %s", err))
	}
	return client
}
