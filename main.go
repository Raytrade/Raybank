package main
import ("context";"os";"github.com/gin-gonic/gin";"github.com/google/uuid";"github.com/jackc/pgx/v5/pgxpool";"github.com/shopspring/decimal")
const Suspense = "ffffffff-ffff-ffff-ffff-ffffffffffff"
func main() {
	ctx := context.Background()
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" { dbUrl = "postgres://localhost:5432/fintech_ledger?sslmode=disable" }
	db, _ := pgxpool.New(ctx, dbUrl)
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Bank-PIN")
		if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }
		c.Next()
	})
	r.GET("/balance/:id", func(c *gin.Context) {
		var b decimal.Decimal
		db.QueryRow(ctx, "SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE account_id = $1", c.Param("id")).Scan(&b)
		c.JSON(200, gin.H{"balance": b.StringFixed(2)})
	})
	r.POST("/transfer/initiate", func(c *gin.Context) {
		var req struct{ From, To, Amount, Idem string }
		c.BindJSON(&req)
		amt, _ := decimal.NewFromString(req.Amount)
		tid := uuid.New().String()
		tx, _ := db.Begin(ctx)
		defer tx.Rollback(ctx)
		tx.Exec(ctx, "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, $3, 'NGN')", tid, req.From, amt.Neg())
		tx.Exec(ctx, "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, $3, 'NGN')", tid, Suspense, amt)
		tx.Exec(ctx, "INSERT INTO transactions (id, idempotency_key, type, status, source_account_id, destination_account_id, amount, currency) VALUES ($1,$2,'p2p','PENDING',$3,$4,$5,'NGN')", tid, req.Idem, req.From, req.To, amt)
		tx.Commit(ctx)
		c.JSON(200, gin.H{"status": "PENDING", "tx_id": tid})
	})
	r.POST("/transfer/settle/:tx_id", func(c *gin.Context) {
		tid := c.Param("tx_id")
		var toAcc string; var amt decimal.Decimal
		db.QueryRow(ctx, "SELECT destination_account_id, amount FROM transactions WHERE id = $1 AND status = 'PENDING'", tid).Scan(&toAcc, &amt)
		tx, _ := db.Begin(ctx)
		defer tx.Rollback(ctx)
		tx.Exec(ctx, "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, $3, 'NGN')", tid, Suspense, amt.Neg())
		tx.Exec(ctx, "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, $3, 'NGN')", tid, toAcc, amt)
		tx.Exec(ctx, "UPDATE transactions SET status = 'SUCCESS' WHERE id = $1", tid)
		tx.Commit(ctx)
		c.JSON(200, gin.H{"status": "SUCCESS"})
	})
	p := os.Getenv("PORT")
	if p == "" { p = "8080" }
	r.Run(":" + p)
}
