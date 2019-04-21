// Package gocheckdeb is to get go packages and check if they are packaged
// for debian or not.
package gocheckdeb

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/willf/pad"
)

// GoDebBinaryStruct is the structere of json
type GoDebBinaryStruct struct {
	Binary         string `json:"binary"`
	XSGoImportPath string `json:"metadata_value"`
	Source         string `json:"source"`
}

// DepMap is the map of dependencies
type DepMap struct {
	deps map[string]DepMap
}

// LevelMap is a single level dependencies map
type LevelMap map[string]string

// StdMap is to store standard packages
var StdMap LevelMap

// GoBinaries is the map of already packaged binaries
var GoBinaries LevelMap

// DepGraph is the graph of packages.
var DepGraph DepMap

const (
	// GoDebBinariesURL is the url of binary list of go lang
	GoDebBinariesURL = "https://api.ftp-master.debian.org/binary/by_metadata/Go-Import-Path"
)

// Err is to log the error
func Err(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// GetGoPath is to get $GOPATH environment variable
func GetGoPath() (string, error) {
	if os.Getenv("GOPATH") == "" {
		return "", errors.New("$GOPATH not set")
	}
	return os.Getenv("GOPATH"), nil
}

// GetProjectPath is to get full project path
func GetProjectPath(project string) (string, error) {
	path, e := GetGoPath()
	if e != nil {
		return "", e
	}
	return path + "/src/" + project, nil
}

// FileExist is check if file exist
func FileExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

// GetURLStatus is to get the status of a package
func GetURLStatus(project string) (bool, error) {
	res, err := http.Get("http://" + project)
	if err != nil {
		return false, errors.New("Can't get " + "http://" + project)
	} else if res.StatusCode >= 200 && res.StatusCode <= 299 {
		return true, nil
	}

	return false, errors.New("Can't get " + "http://" + project)
}

// HandleProject is to get project
func HandleProject(project string) error {
	projectPath, err := GetProjectPath(project)
	if err != nil {
		return err
	}
	// Project is already downloaded
	if FileExist(projectPath) {
		return nil
	}
	// Project don't exist, get it
	if status, err := GetURLStatus(project); status {
		if err != nil {
			return err
		}
		cmd := exec.Command("go", "get", project)
		_, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New("Error in 'go get " + project + "'")
		}
	}

	return nil
}

// GetImports is to get first level dependencies of a project
func GetImports(project, importType string) ([]string, error) {
	cmd := exec.Command("go", "list", "-f", "'{{ join .Imports `\n` }}'", project)
	switch importType {
	case "deps":
		cmd = exec.Command("go", "list", "-f", "'{{ join .Deps `\n` }}'", project)
	case "std":
		cmd = exec.Command("go", "list", "std")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.New("Error in getting 'go list" + importType + "'")
	}

	// Prepare the slice for Output
	libs := strings.Replace(string(out), "'", "", -1)
	slice := strings.Split(libs, "\n")
	return slice, nil
}

// SliceToMap is to convert a slice into a map
func SliceToMap(slice []string) LevelMap {
	m := make(LevelMap)
	for i := 0; i < len(slice); i++ {
		m[slice[i]] = ""
	}
	// Delete the empty elements
	delete(m, "")
	return m
}

// RemoveMap is to remove key of mainMap which are present in needleMap
func RemoveMap(mainMap, needleMap LevelMap) LevelMap {
	for key := range mainMap {
		if _, ok := needleMap[key]; ok {
			delete(mainMap, key)
		}
	}
	return mainMap
}

// MapToSlice is to convert a LevelMap into slice
func MapToSlice(m LevelMap) []string {
	keys := []string{}
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// PrintDep is to print the DepMap
func PrintDep(m DepMap, debFilter bool, i int) {
	for key, value := range m.deps {
		if debFilter {
			if _, ok := GoBinaries[key]; ok {
				fmt.Print(pad.Left("- ", (i+1)*2, " "))
				fmt.Print(key + "\n")
				PrintDep(value, debFilter, i+1)

			} else {
				fmt.Print(pad.Left("- ", (i+1)*2, " "))
				fmt.Print(key)
				fmt.Print(aurora.Bold(aurora.Yellow(" [No Deb Package] \n")))
				PrintDep(value, debFilter, i+1)
			}
		} else {
			fmt.Print(pad.Left("- ", (i+1)*2, " "))
			fmt.Print(aurora.Bold(aurora.Blue(key + "\n")))
			PrintDep(value, debFilter, i+1)
		}

	}
	i++
}

// SliceToDepMap is to convert a slice into a DepMap
func SliceToDepMap(slice []string) DepMap {
	var m DepMap
	m.deps = make(map[string]DepMap)
	for i := 0; i < len(slice); i++ {
		var dummy DepMap
		m.deps[slice[i]] = dummy
	}
	// Delete the empty elements
	delete(m.deps, "")
	return m
}

// GetDep is the on function to get graph or map of dependencies
func GetDep(project string, returnType string) (DepMap, error) {

	stdSlice, err := GetImports("", "std")
	if err != nil {
		var m DepMap
		return m, err
	}
	StdMap = SliceToMap(stdSlice)
	GoBinaries, _ = GetGoDebBinaries()
	DepGraph.deps = make(map[string]DepMap)

	// By default it will give out map or list
	m, err := GetDepRecursive(project, returnType)

	switch returnType {
	case "graph":
		m = DepGraph
	}

	return m, err
}

// GetDepRecursive is to get the recursive map of dependencies
func GetDepRecursive(project string, returnType string) (DepMap, error) {
	// Handle path, if it don't exist, get it.
	HandleProject(project)
	// Convert slice to map, since it's fast in searching.
	importSlice, err := GetImports(project, "imports")
	if err != nil {
		var m DepMap
		return m, err
	}
	importMap := SliceToMap(importSlice)
	// Remove standard libs from users libs
	importMap = RemoveMap(importMap, StdMap)
	// Convert importMap to slice again
	importSlice = MapToSlice(importMap)
	// Convert slice to DepMap now
	importDepMap := SliceToDepMap(importSlice)

	for key := range importDepMap.deps {
		switch returnType {
		case "tree":
			importDepMap.deps[key], _ = GetDepRecursive(key, returnType)
		case "graph":
			DepGraph.deps[key], _ = GetDepRecursive(key, returnType)
		case "list":
			return importDepMap, nil
		default:
			importDepMap.deps[key], _ = GetDepRecursive(key, returnType)
		}
	}

	return importDepMap, nil
}

// SearchDebPackage is to search for a deb package
func SearchDebPackage(name string) bool {
	_, ok := GoBinaries[name]
	return ok
}

// GetGoDebBinaries is to get the complete list of all the binaries packaged in debian
func GetGoDebBinaries() (LevelMap, error) {
	GoBin := make(map[string]string)
	resp, err := http.Get(GoDebBinariesURL)
	var pkgs []GoDebBinaryStruct

	if err != nil {
		return nil, fmt.Errorf("getting %q: %v", GoDebBinariesURL, err)
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		return nil, fmt.Errorf("unexpected HTTP status code: got %d, want %d", got, want)
	}

	if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
		return nil, err
	}

	for _, pkg := range pkgs {
		if !strings.HasSuffix(pkg.Binary, "-dev") {
			continue // skip -dbgsym packages etc.
		}
		for _, importPath := range strings.Split(pkg.XSGoImportPath, ",") {
			// XS-Go-Import-Path can be comma-separated and contain spaces.
			GoBin[strings.TrimSpace(importPath)] = pkg.Binary
		}
	}

	return GoBin, nil
}
