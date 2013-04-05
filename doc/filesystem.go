// Copyright 2011 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package doc

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func GetLocalDoc(rootPath string) ([]*Package, error) {
	var packages []*Package
	filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			panic(err)
		}
		if info.IsDir() && !strings.Contains(path, "testdata") {
			var files []*source
			importPath := strings.Split(path, rootPath)[1]
            pat := path + "/*.go"
			gofiles, err := filepath.Glob(pat)
			if err != nil {
				panic(err)
			}
			for _, file := range gofiles {
				fname := strings.Split(file, rootPath)[1]
				if isDocFile(fname) {
					src := &source{
						name:      fname,
						browseURL: "http://code.google.com/p/go/source/browse/src/pkg/" + importPath + "/" + fname + "?name=release",
						rawURL:    "http://go.googlecode.com/hg-history/release/src/pkg/" + importPath + "/" + fname,
					}
					src.data, err = ioutil.ReadFile(file)
					files = append(files, src)
				}
			}
			b := &builder{
				lineFmt: "#%d",
				pdoc: &Package{
					ImportPath:  importPath,
					ProjectRoot: "",
					ProjectName: "Go",
					ProjectURL:  "https://code.google.com/p/go/",
					BrowseURL:   "http://code.google.com/p/go/source/browse/src/pkg/" + importPath + "?name=release",
					Etag:        "",
					VCS:         "hg",
				},
			}
			pkg, err := b.build(files)
			if err != nil {
				panic(err)
			}
			packages = append(packages, pkg)
		}
		return nil
	})
	return packages, nil
}
