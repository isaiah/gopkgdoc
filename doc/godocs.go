// +build ignore
// Command godocs fetches and prints package documentation.
//
// Usage: go run godocs.go

package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/garyburd/gddo/doc"
	"github.com/garyburd/gosrc"
	_ "github.com/lib/pq"
	godoc "go/doc"
	htemp "html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type timeoutConn struct {
	net.Conn
}

var dialTimeout = 30 * time.Second

func timeoutDial(network, addr string) (net.Conn, error) {
	c, err := net.DialTimeout(network, addr, dialTimeout)
	if err != nil {
		return nil, err
	}
	return &timeoutConn{Conn: c}, nil
}

func main() {
	version := "1.2"
	gosrc.SetLocalDevMode(os.Getenv("GOPATH"))
	pg, err := sql.Open("postgres", "user=isaiah dbname=godock_dev sslmode=disable")
	defer pg.Close()
	if err != nil {
		panic(err)
	}
	_, err = pg.Exec(fmt.Sprintf("delete from functions where version = '%s'", version))
	check(err)
	_, err = pg.Exec(fmt.Sprintf("delete from type_classes where version = '%s'", version))
	check(err)
	_, err = pg.Exec(fmt.Sprintf("delete from namespaces where version = '%s'", version))
	check(err)
	for pkg, _ := range gosrc.GoRepoPath {
		pdoc, err := doc.Get(http.DefaultClient, pkg, "")
		if err != nil {
			fmt.Println("failed to get doc")
			return
		}
		fmt.Println(pdoc.ImportPath)
		store(pdoc, pg, version)
	}
}

func store(pkg *doc.Package, pg *sql.DB, version string) {
	libraryId := 3
	nsSql, err := pg.Prepare("insert into namespaces (name, doc, version, library_id) values ($1, $2, " + version + ", $3) RETURNING id")
	check(err)
	funSql, err := pg.Prepare("insert into functions (name, doc, arglists_comp, version, url_friendly_name, functional_id, functional_type, shortdoc, source, file, line) values ($1, $2, $3, " + version + ", $4, $5, $6, $7, $8, $9, $10) RETURNING id")
	check(err)
	typeSql, err := pg.Prepare("insert into type_classes (name, doc, arglists_comp, type, namespace_id, version, created_at, updated_at, shortdoc) values ($1, $2, $3, 'StructType', $4, " + version + ", $5, $6, $7) RETURNING id")
	check(err)
	exampleSql, err := pg.Prepare("INSERT INTO examples (name, body, doc, output, examplable_id, examplable_type) VALUES ($1, $2, $3, $4, $5, $6)")
	check(err)
	rootPath := "/gopkg/" + version
	var nsId int
	pkgPath := pkg.ImportPath
	err = nsSql.QueryRow(pkgPath, comment(pkg.Doc), libraryId).Scan(&nsId)
	if err != nil {
		panic(err)
	}
	now := time.Now()

	for _, fun := range pkg.Funcs {
		var funId int
		err = funSql.QueryRow(fun.Name, comment(fun.Doc), codeFn(rootPath, pkgPath, fun.Decl), fun.Name, nsId, "Namespace", fun.Decl.Text, sanatize(fun.Source), fun.FileName, strconv.Itoa(fun.Line)).Scan(&funId)
		if err != nil {
			panic(err)
		}
		for _, eg := range fun.Examples {
			_, err = exampleSql.Exec(eg.Name, eg.Code.Text, comment(eg.Doc), eg.Output, funId, "Function")
			check(err)
		}
	}

	for _, t := range pkg.Types {
		var id int
		err = typeSql.QueryRow(t.Name, comment(t.Doc), codeFn(rootPath, pkgPath, t.Decl), nsId, now, now, t.Decl.Text).Scan(&id)
		check(err)
		for _, fun := range t.Funcs {
			var funId int
			err = funSql.QueryRow(fun.Name, comment(fun.Doc), codeFn(rootPath, pkgPath, fun.Decl), fun.Name, id, "TypeClass", fun.Decl.Text, sanatize(fun.Source), fun.FileName, strconv.Itoa(fun.Line)).Scan(&funId)
			if err != nil {
				panic(err)
			}

			for _, eg := range fun.Examples {
				_, err = exampleSql.Exec(eg.Name, eg.Code.Text, comment(eg.Doc), eg.Output, funId, "Function")
				check(err)
			}
		}

		for _, fun := range t.Methods {
			var funId int
			err = funSql.QueryRow(fun.Name, comment(fun.Doc), codeFn(rootPath, pkgPath, fun.Decl), fun.Name, id, "TypeClass", fun.Decl.Text, sanatize(fun.Source), fun.FileName, strconv.Itoa(fun.Line)).Scan(&funId)
			if err != nil {
				panic(err)
			}

			for _, eg := range fun.Examples {
				_, err = exampleSql.Exec(eg.Name, eg.Code.Text, comment(eg.Doc), eg.Output, funId, "Function")
				check(err)
			}
		}

		for _, eg := range t.Examples {
			_, err = exampleSql.Exec(eg.Name, eg.Code.Text, comment(eg.Doc), eg.Output, id, "TypeClass")
			check(err)
		}
	}
}

var (
	h3Open            = []byte("<h3 ")
	h4Open            = []byte("<h4 ")
	h3Close           = []byte("</h3>")
	h4Close           = []byte("</h4>")
	rfcRE             = regexp.MustCompile(`RFC\s+(\d{3,4})`)
	rfcReplace        = []byte(`<a href="http://tools.ietf.org/html/rfc$1">$0</a>`)
	pre               = []byte("<pre")
	shBrush           = []byte("<pre class=\"brush: go\"")
	stopLink          = make(map[string]string)
	linkInCode        = regexp.MustCompile(`(<pre(?:.*?(?:\n))*.*?)<a\s+href="(?:.*?)">(.*?)</a>((?:.*?(?:\n)?.*?)*?</pre>)`)
	linkInCodeReplace = []byte(`$1$2$3`)
)

func comment(v string) string {
	var buf bytes.Buffer
	godoc.ToHTML(&buf, v, stopLink)
	p := buf.Bytes()
	p = bytes.Replace(p, h3Open, h4Open, -1)
	p = bytes.Replace(p, h3Close, h4Close, -1)
	p = bytes.Replace(p, pre, shBrush, -1)
	p = rfcRE.ReplaceAll(p, rfcReplace)
	// rollback the links in code
	//p = linkInCode.ReplaceAll(p, linkInCodeReplace)
	//fmt.Println(string(p))
	return string(p)
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}
func codeFn(rootPath string, pkg string, c doc.Code) string {
	var buf bytes.Buffer
	last := 0
	src := []byte(c.Text)
	for _, a := range c.Annotations {
		htemp.HTMLEscape(&buf, src[last:a.Pos])
		switch a.Kind {
		case doc.PackageLinkAnnotation:
			spew.Dump(a)
			p := rootPath + "/" + c.Paths[a.PathIndex]
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.LinkAnnotation, doc.BuiltinAnnotation:
			p := ""
			if a.PathIndex != -1 {
				p = c.Paths[a.PathIndex]
			}
			if a.Kind == doc.BuiltinAnnotation {
				p = rootPath + "/builtin"
			} else if p != "" {
				p = rootPath + "/" + p
			} else {
				p = rootPath + "/" + pkg
			}
			n := src[a.Pos:a.End]
			n = n[bytes.LastIndex(n, period)+1:]
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteByte('/')
			buf.WriteString(escapePath(string(n)))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.CommentAnnotation:
			buf.WriteString(`<span class="com">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		case doc.AnchorAnnotation:
			buf.WriteString(`<span id="`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</span>`)
		default:
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
		}
		last = int(a.End)
	}
	htemp.HTMLEscape(&buf, src[last:])
	return buf.String()
}

func escapePath(s string) string {
	u := url.URL{Path: s}
	return u.String()
}

var period = []byte{'.'}

func sanatize(s []byte) string {
	return strings.Replace(string(s), string(0x00), "", -1)
}
