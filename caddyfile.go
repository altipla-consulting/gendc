package main

import "html/template"

var tmplCaddyfile = template.Must(template.New("caddyfile").Parse(
	`{
	local_certs
	skip_install_trust
}

{{range $app := .Apps}}
{{range .Domains}}
{{.}}.dev.localhost {
	@grpc protocol grpc
	handle @grpc {
		reverse_proxy {
			to {{$app.Name}}:8080
			transport http {
				versions h2c
			}
		}
	}
	reverse_proxy {{$app.Name}}:8080
	tls /opt/tls/cert.pem /opt/tls/key.pem
}

{{.}}.dev.remote {
	@grpc protocol grpc
	handle @grpc {
		reverse_proxy {
			to {{$app.Name}}:8080
			transport http {
				versions h2c
			}
		}
	}
	reverse_proxy {{$app.Name}}:8080
	tls /opt/tls/cert.pem /opt/tls/key.pem
}
{{end}}
{{end}}

{{range .JS}}
{{.Name}}.dev.localhost {
	reverse_proxy {{.Name}}:3000
	tls /opt/tls/cert.pem /opt/tls/key.pem
}

{{.Name}}.dev.remote {
	reverse_proxy {{.Name}}:3000
	tls /opt/tls/cert.pem /opt/tls/key.pem
}
{{end}}

:80 {
	respond 404
}

:443 {
	respond 404
}
`))
