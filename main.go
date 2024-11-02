package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var (
	users   = []User{}
	nextID  = 1
	mu      sync.Mutex
	client  *mongo.Client
)

// Get all users
func getUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mu.Lock()
	defer mu.Unlock()
	json.NewEncoder(w).Encode(users)
}

// Get a single user by ID
func getUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	for _, user := range users {
		if user.ID == id {
			json.NewEncoder(w).Encode(user)
			return
		}
	}

	http.Error(w, "User not found", http.StatusNotFound)
}

// Create a new user
func createUser(w http.ResponseWriter, r *http.Request, client *mongo.Client, ctx context.Context) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	user.ID = nextID
	nextID++

	collection := client.Database("UsersDB").Collection("Users") 
    _, err := collection.InsertOne(context.TODO(), user)

    if err != nil {
		fmt.Println("Failed to create user: ", user)
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
	}

	users = append(users, user)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)

	fmt.Println("Successfully created user: ", user)
}

// Update an existing user by ID
func updateUser(w http.ResponseWriter, r *http.Request) {
	var updatedUser User
	if err := json.NewDecoder(r.Body).Decode(&updatedUser); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	for i, user := range users {
		if user.ID == id {
			users[i].Name = updatedUser.Name
			users[i].Age = updatedUser.Age
			json.NewEncoder(w).Encode(users[i])
			return
		}
	}

	http.Error(w, "User not found", http.StatusNotFound)
}

// Delete a user by ID
func deleteUser(w http.ResponseWriter, r *http.Request, client *mongo.Client, ctx context.Context) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	id := user.ID

	mu.Lock()
	defer mu.Unlock()

	collection := client.Database("UsersDB").Collection("Users")

    filter := bson.M{"id": id}

    result, err := collection.DeleteOne(ctx, filter)
    if err != nil {
        log.Fatal(err)
    }

    if result.DeletedCount > 0 {
        fmt.Println("User deleted successfully with ID: ", id)
		for i, user := range users {
			if user.ID == id {
				users = append(users[:i], users[i+1:]...)
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
    } else {
        fmt.Println("No User found with the given ID:", id)
    }

	http.Error(w, "User not found", http.StatusNotFound)
}

// Middleware function to enable CORS
func enableCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Set CORS headers
        w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins; replace "*" with a specific origin in production
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS") // Allowed HTTP methods
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization") // Allowed headers

        // Handle preflight OPTIONS request
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        next.ServeHTTP(w, r)
    })
}

func main() {

	// Set up context with a timeout
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
    defer cancel()

	// Connect to MongoDB
    clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
    client, err := mongo.Connect(ctx, clientOptions)
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        if err = client.Disconnect(ctx); err != nil {
            log.Fatal(err)
        }
    }()

	// Check the connection
    err = client.Ping(ctx, nil)
    if err != nil {
        log.Fatal("Could not connect to MongoDB:", err)
    }
    fmt.Println("Connected to MongoDB!")

	// Access the database and collection
    database := client.Database("UsersDB")
    collection := database.Collection("Users")

    // Find all documents in the collection
    cursor, err := collection.Find(ctx, bson.D{})
    if err != nil {
        log.Fatal(err)
    }
    defer cursor.Close(ctx)

    // Iterate through the cursor and print each document
    for cursor.Next(ctx) {
        var user User
        err := cursor.Decode(&user)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println(user)
		users = append(users, user)
    }

    if err := cursor.Err(); err != nil {
        log.Fatal(err)
    }

	fmt.Println("USERS: ", users)

	http.Handle("/users", enableCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getUsers(w, r)
		case http.MethodPost:
			createUser(w, r, client, ctx)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	http.Handle("/user", enableCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getUser(w, r)
		case http.MethodPut:
			updateUser(w, r)
		case http.MethodDelete:
			deleteUser(w, r, client, ctx)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	fmt.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
