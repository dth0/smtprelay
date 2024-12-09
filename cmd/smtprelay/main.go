package main

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

func NewConfig() (*Config, error) {
	cfg := &Config{}

	err := envconfig.Process("smtp", cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
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
	cfg, err := NewConfig()
	if err != nil {
		log.Fatalf("error parsing config: %s\n", err)
	}
	log.Printf("Load config: listen to %s, dkim (%s - %s - %s)", cfg.Listen, cfg.DkimDomain, cfg.DkimSelector, cfg.DkimKey)

	dkimEnabled := true
	privatekey, err := loadPrivateKey(cfg.DkimKey)
	if err != nil {
		log.Printf("error load the privatekey %s: %s\n", cfg.DkimKey, err)
		dkimEnabled = false
	}

	server := &smtpd.Server{
		MaxRecipients: 1,
		Handler: func(_ smtpd.Peer, env smtpd.Envelope) error {
			domain, err := getDomain(env.Recipients[0])
			if err != nil {
				log.Println(err)
				return err
			}

			mx, err := net.LookupMX(domain)
			if err != nil {
				log.Println(err)
				return err
			}

			data := env.Data

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

				data = msg.Bytes()
			}

			for _, hop := range mx {
				host := strings.TrimRight(hop.Host, ".")

				err := smtp.SendMail(fmt.Sprintf("%s:25", host), nil, env.Sender, env.Recipients, data)
				if err == nil {
					log.Printf("Sending email from %s to %s via %s\n", env.Sender, env.Recipients[0], host)
					return nil
				}

			}

			return fmt.Errorf("unable to deliver to: %v", mx)
		},
	}

	go func() {
		log.Printf("Starting server at: (%s)\n", cfg.Listen)
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

func getDomain(email string) (string, error) {
	addr := strings.Split(email, "@")
	if len(addr) != 2 {
		return "", fmt.Errorf("malformed e-mail address: %s", email)
	}

	return addr[1], nil
}
