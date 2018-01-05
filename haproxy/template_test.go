package haproxy

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
)

func TestGetBackendTemplate(t *testing.T) {
	tplt := template.Must(template.New("letter").Parse(GetBackendTemplate()))
	backend := Backend{
		Name: "test-proxy-srv",
		Mode: "http",
		Servers: []Server{
			{Name: "pod1", IP: "10.0.0.1", Port: 80},
			{Name: "pod2", IP: "10.0.0.2", Port: 80},
		},
	}
	buf := &bytes.Buffer{}
	tplt.Execute(buf, backend)
	assert.Equal(t, `
backend test-proxy-srv
	mode	http
	timeout	connect	5s
	timeout	server	5s
	retries	2
	balance	roundrobin
	server	pod1	10.0.0.1:80	check
	server	pod2	10.0.0.2:80	check
`, buf.String())
}
