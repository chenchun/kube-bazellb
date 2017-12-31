package haproxy

func GetSampleTemplate() string {
	return `# haproxy sample from kube-bmlb
global
	maxconn	20000
	ulimit-n	16384
	log	127.0.0.1	local0
	uid	200
	gid	200
	chroot	/var/empty
	nbproc	4
	daemon

listen stats
	bind	:8081
	mode	http
	stats	enable
	stats	hide-version
	stats	realm Haproxy\ Statistics  # Title text for popup window
	stats	uri /
	stats	auth	admin:admin
`
}

func GetFrontendTemplate() string {
	return `
frontend {{.Name}}
	bind	{{.IP}}:{{.Port}}
{{if ne .Mode ""}}	Mode	{{.Mode}}{{end}}
	log	global
	option	httplog
	option	dontlognull
	option	nolinger
	option	http_proxy
	maxconn	8000
	timeout	client	30s
	default_backend	{{.DefaultBackend}}
`
}

type Frontend struct {
	Name           string
	IP             string
	Port           int
	Mode           string
	DefaultBackend string
}

type Backend struct {
	Name    string
	Servers []Server
	Mode    string
}

type Server struct {
	Name string
	IP   string
}

func GetBackendTemplate() string {
	return `
backend {{.Name}}{{if ne .Mode ""}}
	mode	{{.Mode}}{{end}}
	timeout	connect	5s
	timeout	server	5s
	retries	2
	balance	roundrobin{{range .Servers}}
	server	{{.Name}}	{{.IP}}	check{{end}}
`
}
