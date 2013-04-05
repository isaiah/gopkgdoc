package main

import (
	"bytes"
    "net"
	"database/sql"
	"fmt"
	//"github.com/garyburd/gopkgdoc/database"
	"github.com/garyburd/gopkgdoc/doc"
    htemp "html/template"
	_ "github.com/lib/pq"
	godoc "go/doc"
	//"net/http"
	"net/url"
	"regexp"
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
	pg, err := sql.Open("postgres", "user=isaiah dbname=clojuredocs_development sslmode=disable")
	defer pg.Close()
	if err != nil {
		panic(err)
	}
	_, err = pg.Exec("delete from functions where version = '1.1'")
	check(err)
	_, err = pg.Exec("delete from type_classes where version = '1.1'")
	check(err)
	_, err = pg.Exec("delete from namespaces where version = '1.1'")
	check(err)
    root := "/home/isaiah/codes/go/go/src/pkg/"
    pkgs, err := doc.GetLocalDoc(root)
    fmt.Println(len(pkgs))
    for _, pkg := range pkgs {
        fmt.Print(pkg.ImportPath)
        //fmt.Println(len(pkg.Funcs))

        //fmt.Println(pkg)
        store(pkg, pg)
    }
}

func store(pkg *doc.Package, pg *sql.DB) {
      version := "1.1"
      nsSql, err := pg.Prepare("insert into namespaces (name, doc, version, library_id) values ($1, $2, " + version + ", 2) RETURNING id")
      check(err)
      funSql, err := pg.Prepare("insert into functions (name, doc, arglists_comp, version, url_friendly_name, functional_id, functional_type) values ($1, $2, $3, " + version + ", $4, $5, $6)")
      check(err)
      typeSql, err := pg.Prepare("insert into type_classes (name, doc, arglists_comp, type, namespace_id, version, created_at, updated_at) values ($1, $2, $3, 'StructType', $4, " + version + ", $5, $6) RETURNING id")
      check(err)
      var nsId int
      err = nsSql.QueryRow(pkg.ImportPath, comment(pkg.Doc)).Scan(&nsId)
      if err != nil {
          panic(err)
      }
      now := time.Now()

      for _, fun := range pkg.Funcs {
          _, err = funSql.Exec(fun.Name, comment(fun.Doc), codeFn(fun.Decl), fun.Name, nsId, "Namespace")
          if err != nil {
              panic(err)
          }
      }

      for _, t := range pkg.Types {
          var id int
          err = typeSql.QueryRow(t.Name, comment(t.Doc), codeFn(t.Decl), nsId, now, now).Scan(&id)
          check(err)
          for _, fun := range t.Funcs {
              _, err = funSql.Exec(fun.Name, comment(fun.Doc), codeFn(fun.Decl), fun.Name, id, "TypeClass")
              if err != nil {
                  panic(err)
              }
          }

          for _, fun := range t.Methods {
              _, err = funSql.Exec(fun.Name, comment(fun.Doc), codeFn(fun.Decl), fun.Name, id, "TypeClass")
              if err != nil {
                  panic(err)
              }
          }

          //for _, eg := range t.Examples {
          //        fmt.Println(eg.Name)
          //        fmt.Println(eg.Code.Text)
          //        fmt.Println(eg.Doc)
          //        fmt.Println(eg.Output)
          //}
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
func codeFn(c doc.Code) string {
	var buf bytes.Buffer
	last := 0
	src := []byte(c.Text)
	for _, a := range c.Annotations {
		htemp.HTMLEscape(&buf, src[last:a.Pos])
		switch a.Kind {
		case doc.PackageLinkAnnotation:
			p := "/" + a.ImportPath
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteString(`">`)
			htemp.HTMLEscape(&buf, src[a.Pos:a.End])
			buf.WriteString(`</a>`)
		case doc.ExportLinkAnnotation, doc.BuiltinAnnotation:
			p := a.ImportPath
			if a.Kind == doc.BuiltinAnnotation {
				p = "/builtin"
			} else if p != "" {
				p = "/" + p
			}
			n := src[a.Pos:a.End]
			n = n[bytes.LastIndex(n, period)+1:]
			buf.WriteString(`<a href="`)
			buf.WriteString(escapePath(p))
			buf.WriteByte('#')
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

