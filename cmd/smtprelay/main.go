package main

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/smtp"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chrj/smtpd"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/kelseyhightower/envconfig"
)

const DKIMPATH = "/etc/smtprelay/dkim"

type Config struct {
	Listen       string `default:"0.0.0.0:25"`
	DkimKey      string `default:"dkim.key"`
	DkimSelector string `default:"default"`
	DkimDomain   string `default:"example.com"`
}

func init() {
	rand.Seed(time.Now().Unix())
}

// TODO: return a list of server ordered by priority
func findNextHop(domain string) (string, error) {
	entries, err := net.LookupMX(domain)
	if err != nil {
		return "", err
	}

	index := rand.Intn(len(entries))

	host := entries[index].Host

	if host[len(host)-1] == '.' {
		host = host[0 : len(host)-1]
	}

	return host, nil
}

func parseConfig() (Config, error) {
	var c Config
	err := envconfig.Process("smtp", &c)
	if err != nil {
		return c, err
	}

	log.Printf("Load config: listen to %s, dkim (%s - %s - %s)", c.Listen, c.DkimDomain, c.DkimSelector, c.DkimKey)

	return c, err
}

func loadPrivateKey(keyfile string) (crypto.Signer, error) {
	data, err := os.ReadFile(fmt.Sprintf("%s/%s", DKIMPATH, keyfile))
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		log.Fatalf("error parsing config: %s\n", err)
	}

	dkimEnabled := true
	privatekey, err := loadPrivateKey(cfg.DkimKey)
	if err != nil {
		log.Printf("error load the privatekey %s: %s\n", cfg.DkimKey, err)
		dkimEnabled = false
	}

	server := &smtpd.Server{
		MaxRecipients: 1,
		Handler: func(_ smtpd.Peer, env smtpd.Envelope) error {

			domain, err := func(email string) (string, error) {
				addr := strings.Split(email, "@")
				if len(addr) != 2 {
					return "", fmt.Errorf("malformed e-mail address: %s", email)
				}

				return addr[1], nil
			}(env.Recipients[0])

			if err != nil {
				log.Println(err)
				return err
			}

			hop, err := findNextHop(domain)
			if err != nil {
				log.Println(err)
				return err
			}

			log.Printf("Sending email from %s to %s via %s\n", env.Sender, env.Recipients[0], hop)
			if dkimEnabled {
				dkimOpts := &dkim.SignOptions{
					Domain:   cfg.DkimDomain,
					Selector: cfg.DkimSelector,
					Signer:   privatekey,
				}

				var msg bytes.Buffer

				if err := dkim.Sign(&msg, bytes.NewReader(env.Data), dkimOpts); err != nil {
					log.Println(err)
					return err
				}

				return smtp.SendMail(fmt.Sprintf("%s:25", hop), nil, env.Sender, env.Recipients, msg.Bytes())
			}

			return smtp.SendMail(fmt.Sprintf("%s:25", hop), nil, env.Sender, env.Recipients, env.Data)

		},
	}

	go func() {
		log.Printf("Starting server at: %s\n", cfg.Listen)
		err := server.ListenAndServe(cfg.Listen)
		if err != nil && err != smtpd.ErrServerClosed {
			log.Fatalf("error server: %s\n", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server")

	if err := server.Shutdown(true); err != nil {
		log.Fatal(err)
	}
}
