package main
import ("bytes";"context";"encoding/json";"fmt";"net/http";"os";"github.com/gin-gonic/gin";"github.com/google/uuid";"github.com/jackc/pgx/v5/pgxpool";"github.com/shopspring/decimal")
const Suspense = "ffffffff-ffff-ffff-ffff-ffffffffffff"
func main() {
	db, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*"); c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Bank-PIN")
		if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }; c.Next()
	})
	r.StaticFile("/", "./bank.html")
	r.GET("/balance/:id", func(c *gin.Context) {
		var b decimal.Decimal; db.QueryRow(context.Background(), "SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE account_id = $1", c.Param("id")).Scan(&b)
		c.JSON(200, gin.H{"balance": b.StringFixed(2)})
	})
	r.POST("/transfer/initiate", func(c *gin.Context) {
		var req struct{ From, To, Amount, Idem string }; c.BindJSON(&req); amt, _ := decimal.NewFromString(req.Amount); tid := uuid.New().String()
		tx, _ := db.Begin(context.Background()); defer tx.Rollback(context.Background())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, req.From, amt.Neg())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, Suspense, amt)
		tx.Exec(context.Background(), "INSERT INTO transactions (id, idempotency_key, status, source_account_id, destination_account_id, amount) VALUES ($1,$2,'PENDING',$3,$4,$5)", tid, req.Idem, req.From, req.To, amt)
		tx.Commit(context.Background())
		if len(req.To) >= 10 { go callPaystack(req.To, req.Amount) }
		c.JSON(200, gin.H{"status": "PENDING", "tx_id": tid})
	})
	r.POST("/transfer/settle/:tx_id", func(c *gin.Context) {
		tid := c.Param("tx_id"); var to string; var amt decimal.Decimal; db.QueryRow(context.Background(), "SELECT destination_account_id, amount FROM transactions WHERE id = $1", tid).Scan(&to, &amt)
		tx, _ := db.Begin(context.Background()); defer tx.Rollback(context.Background())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, Suspense, amt.Neg())
		tx.Exec(context.Background(), "INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1,$2,$3,'NGN')", tid, to, amt)
		tx.Exec(context.Background(), "UPDATE transactions SET status = 'SUCCESS' WHERE id = $1", tid); tx.Commit(context.Background())
		c.JSON(200, gin.H{"status": "SUCCESS"})
	})
	r.Run(":" + os.Getenv("PORT"))
}
func callPaystack(acc, amt string) {
	key := os.Getenv("PAYSTACK_SECRET"); url1 := "https://api.paystack.co/transferrecipient"
	d1, _ := json.Marshal(map[string]string{"type": "nuban", "name": "OPay User", "account_number": acc, "bank_code": "999992", "currency": "NGN"})
	req1, _ := http.NewRequest("POST", url1, bytes.NewBuffer(d1)); req1.Header.Set("Authorization", "Bearer "+key)
	res1, _ := http.DefaultClient.Do(req1); var r1 struct{ Data struct{ Code string `json:"recipient_code"` } `json:"data"` }
	json.NewDecoder(res1.Body).Decode(&r1)
	url2 := "https://api.paystack.co/transfer"
	d2, _ := json.Marshal(map[string]interface{}{"source": "balance", "amount": 5000, "recipient": r1.Data.Code})
	req2, _ := http.NewRequest("POST", url2, bytes.NewBuffer(d2)); req2.Header.Set("Authorization", "Bearer "+key)
	http.DefaultClient.Do(req2)
}
