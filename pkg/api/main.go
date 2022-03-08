package api

import (
	"fmt"
	"net/http"
	"eth-indexer-practice/pkg/store"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Block = store.Block
type BlockWithoutTransactions = store.BlockWithoutTransactions
type Transaction = store.Transaction

func Run() {
	main()
}

func main() {
	db, err := store.NewPostgres()
	if err != nil {
		panic("failed to connect database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get generic database")
	}
	defer sqlDB.Close()

	r := gin.Default()
	// /blocks?limit=n, default = 10
	r.GET("/blocks", func(c *gin.Context) {
		var blocks []BlockWithoutTransactions
		limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "10"), 10, 64)
		db.Limit(int(limit)).Order("block_num desc").Find(&blocks)

		c.JSON(http.StatusOK, blocks)
	})

	// /blocks/:id
	r.GET("/blocks/:id", func(c *gin.Context) {
		var block Block
		blockNum := c.Param("id")
		err := db.First(&block, "block_num = ?", blockNum).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": fmt.Sprintf("block_num %s not found", blockNum)})
			return
		}

		c.JSON(http.StatusOK, block)
	})

	// /transaction/:txHash
	r.GET("/transaction/:txHash", func(c *gin.Context) {
		var transaction Transaction
		txHash := c.Param("txHash")
		err := db.First(&transaction, "tx_hash = ?", txHash).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": fmt.Sprintf("tx_hash %s not found", txHash)})
			return
		}

		c.JSON(http.StatusOK, transaction)
	})

	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
