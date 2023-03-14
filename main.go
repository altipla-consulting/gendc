package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/kyokomi/emoji/v2"
	log "github.com/sirupsen/logrus"
	"libs.altipla.consulting/collections"
	"libs.altipla.consulting/env"
	"libs.altipla.consulting/errors"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(errors.Stack(err))
	}
}

type configFile struct {
	Services []string     `hcl:"services,optional"`
	Apps     []*configApp `hcl:"app,block"`
	JS       []*configJS  `hcl:"js,block"`
}

type configApp struct {
	Name      string            `hcl:"name,label"`
	DependsOn []string          `hcl:"depends_on,optional"`
	Env       map[string]string `hcl:"env,optional"`
	Source    string            `hcl:"source,optional"`
	Domains   []string          `hcl:"domains,optional"`
}

type configJS struct {
	Name      string   `hcl:"name,label"`
	DependsOn []string `hcl:"depends_on,optional"`
}

func run() error {
	settings := new(configFile)
	if err := hclsimple.DecodeFile("docker-compose.hcl", nil, settings); err != nil {
		return errors.Trace(err)
	}

	for _, app := range settings.Apps {
		if len(app.Domains) == 0 {
			app.Domains = []string{app.Name}
		}
	}

	if err := os.MkdirAll("tmp/gendc", 0700); err != nil {
		return errors.Trace(err)
	}
	if !env.IsCI() {
		if err := createCerts(); err != nil {
			return errors.Trace(err)
		}
	}
	if err := writeDockerCompose(settings); err != nil {
		return errors.Trace(err)
	}
	if err := writeCaddyfile(settings); err != nil {
		return errors.Trace(err)
	}

	log.Println(emoji.Sprintf(":white_check_mark: All configuration files generated successfully!"))

	return nil
}

func createCerts() error {
	if _, err := os.Stat("tmp/gendc/cert.pem"); err != nil && !os.IsNotExist(err) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	log.Info("Generating new local HTTPS certificates")
	gen := exec.Command("mkcert", "-cert-file", "tmp/gendc/cert.pem", "-key-file", "tmp/gendc/key.pem", "*.dev.localhost", "*.dev.remote")
	gen.Stdout = os.Stdout
	gen.Stderr = os.Stderr
	if err := gen.Run(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func writeDockerCompose(settings *configFile) error {
	log.Println("Generating docker-compose.yml file")

	dc := &dcFile{
		Services: make(map[string]*dcService),
	}

	for _, service := range settings.Services {
		switch service {
		case "ravendb":
			dc.Services["ravendb"] = &dcService{
				Image:      "ravendb/ravendb:4.2.104-ubuntu.18.04-x64",
				StopSignal: "SIGKILL",
				Env: map[string]string{
					"RAVEN_Setup_Mode":                      "None",
					"RAVEN_License_Eula_Accepted":           "true",
					"RAVEN_Security_UnsecuredAccessAllowed": "PrivateNetwork",
				},
				Ports: []string{"13000:8080"},
			}

		case "caddy":
			dc.Services["caddy"] = &dcService{
				Image: "caddy:2",
				Ports: []string{"443:443", "80:80"},
				Volumes: []string{
					"./tmp/gendc/Caddyfile:/etc/caddy/Caddyfile",
					"./tmp/gendc:/opt/tls",
				},
			}

		case "postgresql":
			dc.Services["postgresql"] = &dcService{
				Image: "postgres:14",
				Ports: []string{"5432:5432"},
				Env: map[string]string{
					"POSTGRES_USER":     "local",
					"POSTGRES_PASSWORD": "development",
					"POSTGRES_DB":       "local",
				},
			}

		case "sqlserver":
			dc.Services["sqlserver"] = &dcService{
				Image: "mcr.microsoft.com/mssql/server:2019-latest",
				Ports: []string{"1433:1433"},
				Env: map[string]string{
					"ACCEPT_EULA": "Y",
					"SA_PASSWORD": "DevPassword7!",
				},
			}

		case "mysql":
			dc.Services["mysql"] = &dcService{
				Image: "mysql:8.0",
				Ports: []string{"3306:3306"},
				Env: map[string]string{
					"MYSQL_ROOT_PASSWORD": "rootmysql",
					"MYSQL_USER":          "local",
					"MYSQL_PASSWORD":      "development",
					"MYSQL_DATABASE":      "local",
				},
			}

		default:
			return errors.Errorf("unknown service: %s", service)
		}
	}

	if os.Getenv("SSH_AUTH_SOCK") == "" {
		log.Warning("SSH_AUTH_SOCK env variable is not defined. SSH agent is not running right now.")
		log.Warning("You have to configure a SSH agent to run local services.")
		log.Warning("File will be generated but it won't work until the agent is successfully running.")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Trace(err)
	}
	const sshAuthSockEnv = "${SSH_AUTH_SOCK:?Missing SSH_AUTH_SOCK}"
	for _, app := range settings.Apps {
		env := map[string]string{
			"SSH_AUTH_SOCK": sshAuthSockEnv,
			"LOCAL_RAVENDB": "http://ravendb:8080",
			"K_SERVICE":     app.Name,
		}
		for k, v := range app.Env {
			env[k] = v
		}

		if app.Source == "" {
			app.Source = app.Name
		}

		dc.Services[app.Name] = &dcService{
			Image:   "europe-west1-docker.pkg.dev/altipla-tools/devcontainers/go:latest",
			Command: []string{"/usr/local/bin/reloader", "run", ".", "-r", "-e", ".pbtext,.yml,.yaml", "-w", "../pkg", "-w", "../internal", "-w", "../protos"},
			Env:     env,
			Volumes: []string{
				sshAuthSockEnv + ":" + sshAuthSockEnv,
				".:/workspace",
				home + "/go/bin:/go/bin",
				home + "/go/pkg:/go/pkg",
				home + "/.cache/go-build:/home/container/.cache/go-build",
				home + "/.config/gcloud:/home/container/.config/gcloud",
				home + "/.kube:/home/container/.kube",
			},
			User:       os.Getenv("USR_ID") + ":" + os.Getenv("GRP_ID"),
			WorkingDir: "/workspace/" + app.Source,
			DependsOn:  app.DependsOn,
		}
	}

	for _, js := range settings.JS {
		dc.Services[js.Name] = &dcService{
			Image:   "europe-west1-docker.pkg.dev/altipla-tools/devcontainers/node:latest",
			Command: []string{"npm", "start"},
			Env: map[string]string{
				"SSH_AUTH_SOCK": sshAuthSockEnv,
			},
			Volumes: []string{
				sshAuthSockEnv + ":" + sshAuthSockEnv,
				".:/workspace",
			},
			User:       os.Getenv("USR_ID") + ":" + os.Getenv("GRP_ID"),
			WorkingDir: "/workspace",
			DependsOn:  js.DependsOn,
		}
	}

	var buf bytes.Buffer
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "# AUTOGENERATED. DO NOT MODIFY. Run `gendc` to regenerate from `docker-compose.hcl`.")
	fmt.Fprintln(&buf)
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(dc); err != nil {
		return errors.Trace(err)
	}

	if err := ioutil.WriteFile("docker-compose.yml", buf.Bytes(), 0600); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func writeCaddyfile(settings *configFile) error {
	if !collections.HasString(settings.Services, "caddy") {
		return nil
	}

	log.Println("Generating Caddyfile configuration")

	var buf bytes.Buffer
	if err := tmplCaddyfile.Execute(&buf, settings); err != nil {
		return errors.Trace(err)
	}

	if err := ioutil.WriteFile("tmp/gendc/Caddyfile", buf.Bytes(), 0600); err != nil {
		return errors.Trace(err)
	}

	return nil
}
