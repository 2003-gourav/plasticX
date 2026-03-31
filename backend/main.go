package main

import (
    "fmt"
    "net/http"
)

func home(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "PLASTIC backend running")
}

func main() {
    http.HandleFunc("/", home)
    fmt.Println("Server starting on :8080")
    http.ListenAndServe(":8080", nil)
}
