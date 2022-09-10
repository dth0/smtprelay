package main

import (
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
	"github.com/kelseyhightower/envconfig"
)

// TODO: DKIM
type Config struct {
	Listen       string `default:"0.0.0.0:25"`
	DkimCertFile string `default:"cert.pem"`
	DkimKeyFile  string `default:"key.pem"`
	DkimSelector string `default:"default"`
	DkimDomain   string `default:"example.com"`
}

func init() {
	rand.Seed(time.Now().Unix())
}

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

func smtpClientHandler(_ smtpd.Peer, env smtpd.Envelope) error {
	// I know that's weird but I only expect to have one recipient each time
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

	srv, err := findNextHop(domain)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Printf("Sending email from %s to %s via %s\n", env.Sender, env.Recipients[0], srv)
	return smtp.SendMail(fmt.Sprintf("%s:25", srv), nil, env.Sender, env.Recipients, env.Data)
}

func parseConfig() (Config, error) {
	var c Config
	err := envconfig.Process("smtp", &c)
	if err != nil {
		return c, err
	}

	return c, err
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		log.Fatalf("error parsing config: %s\n", err)
	}

	server := &smtpd.Server{
		MaxRecipients: 1,
		Handler:       smtpClientHandler,
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
