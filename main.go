package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
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

//declaracao das variaveis de Blockchain
var Blockchain []Bloco
var tempBlocos []Bloco

//handler de novos blocos para validação
var filaDeBlocos = make(chan Bloco)

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
func geraBloco(blocoAnterior Bloco, dados int, endereco string) (Bloco, error) {
	var novoBloco Bloco

	t := time.Now()

	novoBloco.Indice = blocoAnterior.Indice + 1
	novoBloco.Timestamp = t.String()
	novoBloco.Dados = dados
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

//handler de conexao tcp
func handleConexao(conexao net.Conn) {
	defer conexao.Close()

	go func() {
		for {
			msg := <-anunciador
			io.WriteString(conexao, msg)
		}
	}()
	//endereco do validador
	var endereco string

	/*
		permite a alocacao da quantidade de tokens na stake
		de acordo com o paradigma PoS, quanto maior o número de tokens,
		maior a chance do validador gerar um bloco
	*/
	io.WriteString(conexao, "Entre com o número de Tokens a serem colocados na stake:")
	scanTokens := bufio.NewScanner(conexao)
	for scanTokens.Scan() {
		saldo, err := strconv.Atoi(scanTokens.Text())
		if err != nil {
			log.Printf("%v NaN: %v", scanTokens.Text(), err)
			return
		}
		t := time.Now()
		endereco = calculaHash(t.String())
		validadores[endereco] = saldo
		fmt.Println(validadores)
		break
	}

	io.WriteString(conexao, "\nEntre com os dados:\n")
	scanDados := bufio.NewScanner(conexao)

	go func() {
		for {
			//coleta os dados, valida e adiciona novo bloco
			for scanDados.Scan() {
				dados, err := strconv.Atoi(scanDados.Text())

				if err != nil {
					log.Printf("%v NaN: %v", scanDados.Text(), err)
					mutex.Lock()
					delete(validadores, endereco)
					mutex.Unlock()
					conexao.Close()
				}
				//determina o último indice da blockchain
				mutex.Lock()
				ultimoIndiceAntigo := Blockchain[len(Blockchain)-1]
				mutex.Unlock()

				//cria novo bloco
				novoBloco, err := geraBloco(ultimoIndiceAntigo, dados, endereco)
				if err != nil {
					log.Println(err)
					continue
				}
				if blocoValido(novoBloco, ultimoIndiceAntigo) {
					filaDeBlocos <- novoBloco
				}
				io.WriteString(conexao, "\nEntre com novos dados:\n")
			}
		}
	}()
	//printa o estado do blockchain a cada minuto
	for {
		time.Sleep(30 * time.Second)
		mutex.Lock()
		output, err := json.MarshalIndent(Blockchain, "", "	")
		mutex.Unlock()
		if err != nil {
			log.Fatal(err)
		}
		io.WriteString(conexao, string(output)+"\n")
		log.Println(string(output))
	}
}

//funcao que decide qual validador "vencerá" a validação
func escolheValidador() {
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
		source := rand.NewSource(time.Now().Unix())
		numero := rand.New(source)
		vencedor := loteria[numero.Intn(len(loteria))]

		for _, bloco := range temp {
			if bloco.Validador == vencedor {
				mutex.Lock()
				Blockchain = append(Blockchain, bloco)

				for range validadores {
					anunciador <- "\nValidador do bloco mais atual: " + vencedor + "\n"
				}
				mutex.Unlock()
				break
			}
		}
	}
	mutex.Lock()
	tempBlocos = []Bloco{}
	mutex.Unlock()
}

func criaConexao() (net.Conn, error) {
	conexao, err := net.Dial("tcp", "localhost:"+os.Getenv("ADDR"))
	if err != nil {
		log.Fatal(err)
	}
	return conexao, err
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
	//cria o bloco gênese
	t := time.Now()
	blocoGenese := Bloco{}
	blocoGenese = Bloco{0, t.String(), 0, calculaHashBloco(blocoGenese), "", ""}
	spew.Dump(blocoGenese)
	Blockchain = append(Blockchain, blocoGenese)

	//cria servidor tcp
	server, err := net.Listen("tcp", ":"+os.Getenv("ADDR"))
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	go func() {
		for candidato := range filaDeBlocos {
			mutex.Lock()
			tempBlocos = append(tempBlocos, candidato)
			mutex.Unlock()
		}
	}()

	go func() {
		for {
			escolheValidador()
		}
	}()

	for {
		conexao, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConexao(conexao)
	}
}
