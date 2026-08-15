package main
import ("context";"os";"github.com/gin-gonic/gin";"github.com/google/uuid";"github.com/jackc/pgx/v5/pgxpool";"github.com/shopspring/decimal")
func main() {
	db, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Bank-PIN")
		if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }
		c.Next()
	})
	r.GET("/balance/:id", func(c *gin.Context) {
		var b decimal.Decimal
		db.QueryRow(context.Background(), "SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE account_id = $1", c.Param("id")).Scan(&b)
		c.JSON(200, gin.H{"balance": b.StringFixed(2)})
	})
	r.POST("/transfer/initiate", func(c *gin.Context) {
		var req struct{ From, To, Amount, Idem string }
		c.BindJSON(&req); amt, _ := decimal.NewFromString(req.Amount); tid := uuid.New().String()
		tx, _ := db.Begin(context.Background()); defer tx.Rollback(context.Background())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, req.From, amt.Neg())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, "ffffffff-ffff-ffff-ffff-ffffffffffff", amt)
		tx.Exec(context.Background(), "INSERT INTO transactions (id, idempotency_key, status, source_account_id, destination_account_id, amount) VALUES ($1,$2,'PENDING',$3,$4,$5)", tid, req.Idem, req.From, req.To, amt)
		tx.Commit(context.Background())
		c.JSON(200, gin.H{"status": "PENDING", "tx_id": tid})
	})
	r.POST("/transfer/settle/:tx_id", func(c *gin.Context) {
		tid := c.Param("tx_id"); var to string; var amt decimal.Decimal
		db.QueryRow(context.Background(), "SELECT destination_account_id, amount FROM transactions WHERE id = $1", tid).Scan(&to, &amt)
		tx, _ := db.Begin(context.Background()); defer tx.Rollback(context.Background())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, "ffffffff-ffff-ffff-ffff-ffffffffffff", amt.Neg())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, to, amt)
		tx.Exec(context.Background(), "UPDATE transactions SET status = 'SUCCESS' WHERE id = $1", tid)
		tx.Commit(context.Background())
		c.JSON(200, gin.H{"status": "SUCCESS"})
	})
	p := os.Getenv("PORT")
	if p == "" { p = "8080" }
	r.Run(":" + p)
}
