/*
Copyright © 2026 Matt Krueger <mkrueger@rstms.net>
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

 1. Redistributions of source code must retain the above copyright notice,
    this list of conditions and the following disclaimer.

 2. Redistributions in binary form must reproduce the above copyright notice,
    this list of conditions and the following disclaimer in the documentation
    and/or other materials provided with the distribution.

 3. Neither the name of the copyright holder nor the names of its contributors
    may be used to endorse or promote products derived from this software
    without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.
*/
package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var dependsCmd = &cobra.Command{
	Use:   "depends FILENAME",
	Short: "walk DLL dependency tree",
	Long: `
Use dumpbin.exe from the Microsoft Visual Studio build tools to scan
FILENAME generating a list of DLL dependencies.
Recursively scan each DLL until no further dependencies are found. 
Sort and dedup the resulting list.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ViperSetDefault("dumpbin", "dumpbin.exe")
		ViperSetDefault("re_begin", `^[[:space:]]*Image has the following dependencies:[[:space:]]*$`)
		ViperSetDefault("re_end", `^[[:space:]]*Summary[[:space:]]*$`)
		results, err := WalkDependencies(filepath.Clean(args[0]))
		cobra.CheckErr(err)
		fmt.Println(strings.Join(*results, "\n"))
	},
}

func init() {
	CobraAddCommand(rootCmd, rootCmd, dependsCmd)
}

type Walker struct {
	dumpbin string
	results map[string]string
	begin   *regexp.Regexp
	end     *regexp.Regexp
	winpath string
	syspath string
	debug   bool
}

func NewWalker() (*Walker, error) {
	w := Walker{
		dumpbin: ViperGetString("dumpbin"),
		results: map[string]string{},
		debug:   ViperGetBool("debug"),
	}
	if !IsFile(w.dumpbin) {
		return nil, Fatalf("dumpbin '%s' is not a file", w.dumpbin)
	}
	var err error
	w.begin, err = regexp.Compile(ViperGetString("re_begin"))
	if err != nil {
		return nil, Fatal(err)
	}
	w.end, err = regexp.Compile(ViperGetString("re_end"))
	if err != nil {
		return nil, Fatal(err)
	}
	w.winpath, err = windows.GetWindowsDirectory()
	if err != nil {
		return nil, Fatal(err)
	}
	w.winpath = strings.ToLower(filepath.Clean(w.winpath))
	w.syspath, err = windows.GetSystemDirectory()
	if err != nil {
		return nil, Fatal(err)
	}
	w.syspath = strings.ToLower(filepath.Clean(w.syspath))
	return &w, nil
}

func logOutput(ofp io.Writer, label string, buf *bytes.Buffer) {
	output := buf.String()
	output = strings.TrimSpace(output)
	if output != "" {
		fmt.Fprintf(ofp, "%s: %s\n", label, output)
	}
}

func (w *Walker) scan(filename string) (*[]string, error) {
	_, dumper := filepath.Split(w.dumpbin)
	cmd := exec.Command(w.dumpbin, "/DEPENDENTS", filename)
	var obuf, ebuf bytes.Buffer
	cmd.Stdout = &obuf
	cmd.Stderr = &ebuf
	if w.debug {
		log.Printf("executing: %s\n", cmd)
	}
	err := cmd.Run()
	if err != nil {
		logOutput(os.Stderr, "stderr", &ebuf)
		switch err.(type) {
		case *exec.ExitError:
			exitErr := err.(*exec.ExitError)
			fmt.Fprintf(os.Stderr, "ExitError.Stderr: %s\n", string(exitErr.Stderr))
			fmt.Fprintf(os.Stderr, "ExitError.ProcessState: %s\n", exitErr.ProcessState.String())
			return nil, Fatalf("process %s exited %d", dumper, exitErr.ProcessState.ExitCode())
		default:
			return nil, Fatal(err)
		}
	}
	lines, err := w.parseOutput(&obuf)
	if err != nil {
		return nil, Fatal(err)
	}
	return lines, nil
}

func (w *Walker) parseOutput(buf *bytes.Buffer) (*[]string, error) {
	var active bool
	lines := []string{}
	scanner := bufio.NewScanner(buf)
	for {
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
		case !active && w.begin.MatchString(line):
			active = true
		case active && w.end.MatchString(line):
			active = false
		case active && !slices.Contains(lines, line):
			lines = append(lines, line)
		}
	}
	err := scanner.Err()
	if err != nil {
		return nil, Fatal(err)
	}
	return &lines, nil
}

func (w *Walker) walk(filename string) error {
	_, basename := filepath.Split(filename)
	depNames, err := w.scan(filename)
	if err != nil {
		return Fatal(err)
	}
	for _, depName := range *depNames {
		_, seen := w.results[depName]
		if seen {
			if w.debug {
				log.Printf("DUP: base=%s name=%s\n", basename, depName)
			}
		} else {
			depPath, err := GetDLLPath(depName)
			if err != nil {
				return Fatal(err)
			}
			depDir, depFile := filepath.Split(depPath)
			depDir = strings.ToLower(filepath.Clean(depDir))
			switch depDir {
			case w.winpath, w.syspath:
				if w.debug {
					log.Printf("SYS: base=%s name=%s file=%s path=%s\n", basename, depName, depFile, depPath)
				}
			default:
				if w.debug {
					log.Printf("NEW: base=%s name=%s file=%s path=%s\n", basename, depName, depFile, depPath)
				}
				w.results[depName] = depPath
				err = w.walk(depPath)
				if err != nil {
					return Fatal(err)
				}
			}
		}
	}
	return nil
}

func WalkDependencies(filename string) (*[]string, error) {
	if !IsFile(filename) {
		return nil, Fatalf("exefile: '%s' is not a file", filename)
	}
	walker, err := NewWalker()
	if err != nil {
		return nil, Fatal(err)
	}
	err = walker.walk(filename)
	if err != nil {
		return nil, Fatal(err)
	}
	paths := []string{}
	for _, path := range walker.results {
		//fmt.Printf("%s: %s\n", name, path)
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return &paths, nil
}

func GetDLLPath(filename string) (string, error) {
	hModule, err := windows.LoadLibrary(filename)
	if err != nil {
		return "", Fatal(err)
	}
	defer func() {
		err := windows.FreeLibrary(hModule)
		if err != nil {
			panic(fmt.Sprintf("%v", Fatal(err)))
		}

	}()
	var buf [windows.MAX_PATH]uint16
	ret, err := windows.GetModuleFileName(hModule, &buf[0], uint32(len(buf)))
	if ret == 0 {
		return "", Fatal(err)
	}
	dllPath := windows.UTF16ToString(buf[:])
	return dllPath, nil
}
