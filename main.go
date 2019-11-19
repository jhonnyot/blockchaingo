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

/*
• Constantes
*/
const dificuldade = 5
const numeroMaximoThreads = 1
const numeroMaximoBlocos = 5
const contador = 1E7

// declaracao de bloco
type Bloco struct {
	Indice      int
	Timestamp   string
	Dados       int
	Hash        string
	HashAnt     string
	Dificuldade int
	Nonce       string
}

// declaracao de blockchain
var Blockchain []Bloco

// mensagem para handling post/get
type Mensagem struct {
	Dados int
}

// mutex para garantir nao-concorrencia e evitar acessos simultaneos
var mutex = &sync.Mutex{}

//main
func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		t := time.Now()
		blocoGenese := Bloco{}
		blocoGenese = Bloco{0, t.String(), 0, calculaHash(blocoGenese), "", dificuldade, ""}
		spew.Dump(blocoGenese)

		mutex.Lock()
		Blockchain = append(Blockchain, blocoGenese)
		mutex.Unlock()
	}()
	log.Fatal(run())
}

//funcao que cria e configura o servlet
func run() error {
	mux := makeMuxRouter()
	httpAddr := os.Getenv("ADDR")
	log.Println("Servlet ouvindo na porta ", httpAddr)
	server := &http.Server{
		Addr:           ":" + httpAddr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if err := server.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

//funcao que cria o roteador
func makeMuxRouter() http.Handler {
	muxRouter := mux.NewRouter()
	muxRouter.HandleFunc("/", handleGetBlockchain).Methods("GET")
	muxRouter.HandleFunc("/", handleEscreveBloco).Methods("POST")
	return muxRouter
}

//handler do blockchain
func handleGetBlockchain(writer http.ResponseWriter, req *http.Request) {
	bytes, err := json.MarshalIndent(Blockchain, "", "  ")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(writer, string(bytes))
}

//handler de bloco (escreve novo bloco)
func handleEscreveBloco(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	var m Mensagem

	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&m); err != nil {
		respondWithJSON(writer, req, http.StatusBadRequest, req.Body)
		return
	}
	defer req.Body.Close()

	//garante atomicidade ao criar o bloco
	mutex.Lock()

	novoBloco := geraBloco(Blockchain[len(Blockchain)-1], m.Dados)

	//desfaz o lock
	mutex.Unlock()

	if blocoValido(novoBloco, Blockchain[len(Blockchain)-1]) {
		Blockchain = append(Blockchain, novoBloco)
		spew.Dump(novoBloco)
	}

	respondWithJSON(writer, req, http.StatusCreated, novoBloco)
}

//validador da corrente
func blocoValido(novoBloco, blocoAnterior Bloco) bool {
	if blocoAnterior.Indice+1 != novoBloco.Indice {
		return false
	}
	if blocoAnterior.Hash != novoBloco.HashAnt {
		return false
	}
	if calculaHash(novoBloco) != novoBloco.Hash {
		return false
	}

	return true
}

//calculadora de hashes
func calculaHash(bloco Bloco) string {
	totalDados := strconv.Itoa(bloco.Indice) + bloco.Timestamp + strconv.Itoa(bloco.Dados) + bloco.HashAnt + bloco.Nonce
	hasher := sha256.New()
	hasher.Write([]byte(totalDados))
	hashFinal := hasher.Sum(nil)
	return hex.EncodeToString(hashFinal)
}

//validador de hash
func validaHash(hash string, dificuldade int) bool {
	prefixo := strings.Repeat("0", dificuldade)
	return strings.HasPrefix(hash, prefixo)
}

//gerador de blocos
func geraBloco(blocoAntigo Bloco, dados int) Bloco {
	var novoBloco Bloco

	t := time.Now()

	novoBloco.Indice = blocoAntigo.Indice + 1
	novoBloco.Timestamp = t.String()
	novoBloco.Dados = dados
	novoBloco.HashAnt = blocoAntigo.Hash
	novoBloco.Dificuldade = dificuldade

	for i := 0; ; i++ {
		hex := fmt.Sprintf("%x", i)
		novoBloco.Nonce = hex
		if !validaHash(calculaHash(novoBloco), novoBloco.Dificuldade) {
			if i%contador == 0 {
				fmt.Println(i)
			}
			// fmt.Println(calculaHash(novoBloco), " invalido.")
			// time.Sleep(time.Millisecond)
			continue
		} else {
			fmt.Println(calculaHash(novoBloco), " valido.")
			novoBloco.Hash = calculaHash(novoBloco)
			break
		}
	}
	return novoBloco
}

//handler de erro
func respondWithJSON(writer http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	response, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte("HTTP 500: Internal Server Error"))
		return
	}
	writer.WriteHeader(code)
	writer.Write(response)
}
