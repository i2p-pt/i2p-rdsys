package main

import (
	"flag"
	"log"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	gettorMail "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/gettor"
	httpsUI "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/https"
	moatWeb "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/moat"
	salmonWeb "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/salmon"
	stubWeb "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/stub"
	telegramBot "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/telegram"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/gettor"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/https"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/moat"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/salmon"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/stub"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/telegram"
)

func main() {
	var configFilename, distName string
	flag.StringVar(&distName, "name", "", "Distributor name.")
	flag.StringVar(&configFilename, "config", "", "Configuration file.")
	flag.Parse()

	if distName == "" {
		log.Fatal("No distributor name provided.  The argument -name is mandatory.")
	}

	if configFilename == "" {
		log.Fatal("No configuration file provided.  The argument -config is mandatory.")
	}
	cfg, err := internal.LoadConfig(configFilename)
	if err != nil {
		log.Fatal(err)
	}

	var constructors = map[string]func(*internal.Config){
		salmon.DistName:   salmonWeb.InitFrontend,
		https.DistName:    httpsUI.InitFrontend,
		stub.DistName:     stubWeb.InitFrontend,
		gettor.DistName:   gettorMail.InitFrontend,
		moat.DistName:     moatWeb.InitFrontend,
		telegram.DistName: telegramBot.InitFrontend,
	}
	runFunc, exists := constructors[distName]
	if !exists {
		log.Fatalf("Distributor %q not found.", distName)
	}

	log.Printf("Starting distributor %q.", distName)
	runFunc(cfg)
}
