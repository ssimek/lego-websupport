package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"
)

type Records struct {
	Items []Record
}

type Record struct {
	Type    string `json:"type"`
	ID      int64  `json:"id,omitempty"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl,omitempty"`
	Note    string `json:"note,omitempty"`
}

func env(key string, defVal string) string {
	val, found := os.LookupEnv(key)
	if !found {
		val = defVal
	}
	return val
}

func wsapi(method string, url string, body io.Reader) []byte {
	apiRoot := env("WEBSUPPORT_API_ROOT", "https://rest.websupport.sk")
	apiKey := env("WEBSUPPORT_API_KEY", "")
	apiSecret := env("WEBSUPPORT_API_SECRET", "")

	now := time.Now().UTC()

	hash := hmac.New(sha1.New, []byte(apiSecret))
	canonical := fmt.Sprintf("%s %s %d", method, url, now.Unix())
	log.Println("WSAPI >> ", canonical)
	hash.Write([]byte(canonical))
	sig := hex.EncodeToString(hash.Sum(nil))

	client := &http.Client{}
	req, err := http.NewRequest(method, apiRoot+url, body)
	if err != nil {
		log.Fatal(err)
	}

	req.SetBasicAuth(apiKey, sig)
	req.Header.Set("Date", now.Format(time.RFC3339))

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode/100 != 2 {
		log.Fatalf("WSAPI !! %d %s", resp.StatusCode, resp.Status)
	}

	return data
}

func main() {
	var args []string

	for _, arg := range os.Args[1:] {
		if arg != "--" {
			args = append(args, arg)
		}
	}

	if len(args) < 3 {
		log.Fatal("3 arguments expected", args)
	}

	command := args[0]
	token := args[2]
	rexBaseDomain, _ := regexp.Compile("(.+)\\.([^.]+\\.[^.]+)\\.$")
	matches := rexBaseDomain.FindStringSubmatch(args[1])
	domain := matches[1]
	base := matches[2]

	switch command {
	case "present":
		data, err := json.Marshal(Record{
			Type:    "TXT",
			Name:    domain,
			Content: token,
			TTL:     10,
		})
		if err != nil {
			log.Fatal(err)
		}
		wsapi("POST", "/v1/user/self/zone/"+base+"/record", bytes.NewBuffer(data))
		break

	case "cleanup":
		var records Records

		err := json.Unmarshal(wsapi("GET", "/v1/user/self/zone/"+base+"/record", nil), &records)
		if err != nil {
			log.Fatal(err)
		}

		var found *Record = nil

		for _, record := range records.Items {
			if record.Type == "TXT" && record.Name == domain && record.Content == token {
				found = &record
				log.Printf("Deleting record ID %d", record.ID)
				wsapi("DELETE", fmt.Sprintf("/v1/user/self/zone/%s/record/%d", base, record.ID), nil)
				break
			}
		}

		if found == nil {
			log.Printf("Record %s for domain %s not found", token, domain)
		}
		break

	default:
		log.Fatalf("Unknown command: %s", command)
		break
	}
}
