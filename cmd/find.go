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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var findCmd = &cobra.Command{
	Version: "0.0.1",
	Use:     "find FILENAME",
	Short:   "find installed exe files",
	Long: `
Scan the installed program directories for files named FILENAME.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		results, err := FindExecutable(args[0], nil)
		cobra.CheckErr(err)
		if results != nil {
			fmt.Println(strings.Join(*results, "\n"))
		}
	},
}

func init() {
	CobraAddCommand(rootCmd, rootCmd, findCmd)
}

func FindExecutable(filename string, root *string) (*[]string, error) {
	debug := ViperGetBool("debug")
	results := map[string]bool{}
	if root == nil {
		for _, line := range os.Environ() {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				switch strings.ToLower(key) {
				case "programfiles", "programfiles(x86)":
					files, err := FindExecutable(strings.ToLower(filename), &value)
					if err != nil {
						return nil, Fatal(err)
					}
					for _, file := range *files {
						results[filepath.Clean(file)] = true
					}
				}
			}
		}
	} else {
		err := filepath.WalkDir(*root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil {
					if d.IsDir() {
						if debug {
							fmt.Fprintf(os.Stderr, "directory: %s: %s\n", path, Fatal(err))
						}
						return fs.SkipDir
					}
				}
				return err
			}
			if !d.IsDir() {
				if strings.ToLower(d.Name()) == filename {
					results[filepath.Clean(path)] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, Fatal(err)
		}
	}
	files := []string{}
	for file, _ := range results {
		files = append(files, file)
	}
	sort.Strings(files)
	return &files, nil
}
