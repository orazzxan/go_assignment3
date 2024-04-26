package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/lib/pq"
)

type Product struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

var (
	db  *sql.DB
	rdb *redis.Client
)

func main() {
	var err error
	db, err = sql.Open("postgres", "postgres://username:123123@localhost/db?sslmode=disable")
	if err != nil {
		log.Fatal("Error connecting to the database:", err)
	}
	defer db.Close()

	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis-10450.c8.us-east-1-4.ec2.redns.redis-cloud.com:10450",
		Password: "k2Qs5tj6eOUnmDdyM26V2FdKlsHp1ETS",
		DB:       0,
	})

	_, err = rdb.Ping(rdb.Context()).Result()
	if err != nil {
		log.Fatal("Error connecting to Redis:", err)
	}

	http.HandleFunc("/product", getProduct)
	http.HandleFunc("/create-product", createProduct)

	log.Println("Server listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getProduct(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Product ID is required", http.StatusBadRequest)
		return
	}

	val, err := rdb.Get(rdb.Context(), id).Result()
	if err == nil {
		var product Product
		err := json.Unmarshal([]byte(val), &product)
		if err != nil {
			http.Error(w, "Error unmarshalling product data", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(product)
		return
	} else if err != redis.Nil {
		http.Error(w, "Error checking Redis cache", http.StatusInternalServerError)
		return
	}

	row := db.QueryRow("SELECT id, name, description, price FROM products WHERE id = $1", id)
	var product Product
	err = row.Scan(&product.ID, &product.Name, &product.Description, &product.Price)
	if err == sql.ErrNoRows {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Error fetching product from database", http.StatusInternalServerError)
		return
	}

	productJSON, err := json.Marshal(product)
	if err != nil {
		http.Error(w, "Error marshalling product data", http.StatusInternalServerError)
		return
	}
	err = rdb.Set(rdb.Context(), id, productJSON, 5*time.Minute).Err()
	if err != nil {
		http.Error(w, "Error caching product data in Redis", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(product)
}

func createProduct(w http.ResponseWriter, r *http.Request) {
	var product Product
	err := json.NewDecoder(r.Body).Decode(&product)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err = db.Exec("INSERT INTO products (name, description, price) VALUES ($1, $2, $3)", product.Name, product.Description, product.Price)
	if err != nil {
		http.Error(w, "Error inserting product into database", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(product)
}
