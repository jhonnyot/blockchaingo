package main

import (
	// "bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	// "fmt"
	"io"
	"log"
	"math/rand"
	// "net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

//declaracao da estrutura de Bloco
type Bloco struct {
	Indice    int
	Timestamp string
	Dados     int
	Hash      string
	HashAnt   string
	Validador string
}

//carteira
type Carteira struct {
	ID     uuid.UUID `json:"id"`
	Stakes []Stake   `json:"stakes"`
}

//stake
type Stake struct {
	ID         uuid.UUID `json:"id"`
	IDCarteira uuid.UUID `json:"idcarteira"`
	Dados      int       `json:"dados"`
	Tokens     int       `json:"tokens"`
}

//declaracao das variaveis de Blockchain
var Blockchain []Bloco
var tempBlocos []Bloco

//handler de novos blocos para validação
var filaDeBlocos []Bloco

//validadores
var validadores = make(map[string]int)

//variavel que anuncia novos blocos válidos para os nós
var anunciador = make(chan string)

//sincronizador; garante não-concorrência nas adições de blocos
var mutex = &sync.Mutex{}

//calculador de hashes
func calculaHash(s string) string {
	hasher := sha256.New()
	hasher.Write([]byte(s))
	hashFinal := hasher.Sum(nil)
	return hex.EncodeToString(hashFinal)
}

//calculador de hashes do bloco
func calculaHashBloco(bloco Bloco) string {
	totalDados := string(bloco.Indice) + bloco.Timestamp + string(bloco.Dados) + bloco.HashAnt
	return calculaHash(totalDados)
}

//gerador de novos blocos
func geraBloco(blocoAnterior Bloco, endereco string, stake Stake) (Bloco, error) {
	var novoBloco Bloco

	t := time.Now()

	novoBloco.Indice = blocoAnterior.Indice + 1
	novoBloco.Timestamp = t.String()
	novoBloco.Dados = stake.Dados
	novoBloco.HashAnt = blocoAnterior.Hash
	novoBloco.Hash = calculaHashBloco(novoBloco)
	novoBloco.Validador = endereco

	return novoBloco, nil
}

//validador da corrente
func blocoValido(novoBloco, blocoAnterior Bloco) bool {
	if blocoAnterior.Indice+1 != novoBloco.Indice {
		return false
	}
	if blocoAnterior.Hash != novoBloco.HashAnt {
		return false
	}
	if calculaHashBloco(novoBloco) != novoBloco.Hash {
		return false
	}

	return true
}

//funcao que decide qual validador "vencerá" a validação
func escolheValidador() {
	spew.Dump("Validando")
	go func() {
		mutex.Lock()
		for _, candidato := range filaDeBlocos {
			tempBlocos = append(tempBlocos, candidato)
		}
		filaDeBlocos = []Bloco{}
		mutex.Unlock()
	}()
	spew.Dump(filaDeBlocos)
	time.Sleep(3 * time.Second)
	mutex.Lock()
	temp := tempBlocos
	mutex.Unlock()

	loteria := []string{}
	if len(temp) > 0 {
		//percorre o slice de blocos procurando validadores únicos
	EXTERNO:
		for _, bloco := range temp {
			for _, node := range loteria {
				if bloco.Validador == node {
					continue EXTERNO
				}
			}

			//persiste validadores
			mutex.Lock()
			setValidadores := validadores
			mutex.Unlock()

			//para cada token do validador na stake, insere a identificacao deste validador na loteria
			k, ok := setValidadores[bloco.Validador]
			if ok {
				for i := 0; i < k; i++ {
					loteria = append(loteria, bloco.Validador)
				}
			}
		}

		//escolhe um vencedor aleatório
		if len(loteria) > 0 {
			source := rand.NewSource(time.Now().Unix())
			numero := rand.New(source)
			vencedor := loteria[numero.Intn(len(loteria))]

			for _, bloco := range temp {
				if bloco.Validador == vencedor {
					mutex.Lock()
					Blockchain = append(Blockchain, bloco)
					delete(validadores, bloco.Validador)
					loteria = []string{}
					spew.Dump(Blockchain)
					mutex.Unlock()
					break
				}
			}
		}

	}
	mutex.Lock()
	tempBlocos = []Bloco{}
	mutex.Unlock()
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
	var stake Stake

	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&stake); err != nil {
		respondWithJSON(writer, req, http.StatusBadRequest, req.Body)
		return
	}
	defer req.Body.Close()

	endereco := calculaHash(stake.ID.String())
	validadores[endereco] = stake.Tokens
	spew.Dump(validadores)

	//determina o último indice da blockchain
	mutex.Lock()
	ultimoIndiceAntigo := Blockchain[len(Blockchain)-1]
	mutex.Unlock()

	//cria novo bloco
	novoBloco, err := geraBloco(ultimoIndiceAntigo, endereco, stake)
	if err != nil {
		log.Println(err)
	}
	respondWithJSON(writer, req, http.StatusCreated, novoBloco)
	if blocoValido(novoBloco, ultimoIndiceAntigo) {
		filaDeBlocos = append(filaDeBlocos, novoBloco)
	}
}

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

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		t := time.Now()
		blocoGenese := Bloco{}
		blocoGenese = Bloco{0, t.String(), 0, calculaHashBloco(blocoGenese), "", ""}
		spew.Dump(blocoGenese)

		mutex.Lock()
		Blockchain = append(Blockchain, blocoGenese)
		mutex.Unlock()
	}()

	go func() {
		time.Sleep(10 * time.Second)
		for {
			escolheValidador()
		}
	}()
	log.Fatal(run())
}

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
