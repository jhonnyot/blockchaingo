package main

import (
        "crypto/sha256"
        "encoding/hex"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "os"
        "strconv"
        "strings"
        "sync"
        "time"

        "github.com/davecgh/go-spew/spew"
        "github.com/gorilla/mux"
        "github.com/joho/godotenv"
)

const DIFICULDADE = 1

type Bloco struct {
	Indice 		int
	Timestamp	string
	Dados		int
	Hash		string
	HashAnt		string
	Dificuldade	int
	Nonce		string
}

var Blockchain []Bloco
