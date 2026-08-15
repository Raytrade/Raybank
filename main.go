package main
import ("bytes";"context";"encoding/json";"fmt";"net/http";"os";"github.com/gin-gonic/gin";"github.com/google/uuid";"github.com/jackc/pgx/v5/pgxpool";"github.com/shopspring/decimal")
const MyAccount = "c0a80001-0000-0000-0000-000000000001"
const Suspense = "ffffffff-ffff-ffff-ffff-ffffffffffff"
func main() {
	db, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Bank-PIN, x-goog-api-key")
		if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }; c.Next()
	})
	r.StaticFile("/", "./bank.html")
	r.GET("/balance/:id", func(c *gin.Context) {
		var b decimal.Decimal; db.QueryRow(context.Background(), "SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE account_id = $1", c.Param("id")).Scan(&b)
		c.JSON(200, gin.H{"balance": b.StringFixed(2)})
	})
	r.POST("/brain", func(c *gin.Context) {
		var req struct{ Prompt string `json:"input"` }; c.BindJSON(&req)
		url := "https://generativelanguage.googleapis.com/v1beta/interactions"
		payload, _ := json.Marshal(map[string]interface{}{"model": "gemini-1.5-flash", "input": "You are Raybank AI. The user says: " + req.Prompt})
		aiReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		aiReq.Header.Set("x-goog-api-key", os.Getenv("GEMINI_API_KEY"))
		aiReq.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(aiReq)
		var result map[string]interface{}; json.NewDecoder(resp.Body).Decode(&result)
		c.JSON(200, result)
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
	r.Run(":" + os.Getenv("PORT"))
}
func callPaystack(acc, amt string) {
	key := os.Getenv("PAYSTACK_SECRET"); url := "https://api.paystack.co/transferrecipient"
	d, _ := json.Marshal(map[string]string{"type": "nuban", "name": "OPay User", "account_number": acc, "bank_code": "999992"})
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(d)); req.Header.Set("Authorization", "Bearer "+key)
	http.DefaultClient.Do(req)
}
