package main

import (
	"bob-index/database"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var M = flag.String("M", "search", "Mode selection (clean, predir, scan, search, add, delete) search is default")
var D = flag.String("D", "/ftp-data/bob/bob-index.db", "Location of database")
var G = flag.String("G", "/home/ftpd/glftpd", "gl root path")
var P = flag.String("P", "/mp3", "Scan path (inside glroot)")
var L = flag.Int("L", 50, "Limit number of search results")
var s = flag.String("s", "test", "search string")
var p = flag.String("p", "/private/", "path for individual add or delete")
var n = flag.String("n", "test", "name of release for individual add or delete")
var c = flag.Bool("c", false, "case sensitivity (we dont care by default)")
var d = flag.Bool("d", false, "debug")

func addentry(path string, name string) {
	result, err := database.DBCon.Exec("INSERT or IGNORE INTO release(path, lower, name) VALUES (?, ?, ?)", path, strings.ToLower(name), name)
	if err != nil {
		log.Fatal(err)
	}
	nAffected, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}
	if (nAffected != 0) && (*d == true) {
		fmt.Printf("INSERT %s\n", path)
	}

}

func delentry(path string) {
	result, err := database.DBCon.Exec("DELETE FROM release WHERE path = ?", path)
	if err != nil {
		log.Fatal(err)
	}
	nAffected, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}
	if (nAffected != 0) && (*d == true) {
		fmt.Printf("DELETE %s\n", path)
	}

}

func clean(glroot string) {
	fmt.Printf("Cleaning up database at %v\n", *G+*D)
	// Compile a slice with all release paths that no longer exist
	rows, err := database.DBCon.Query("SELECT path FROM release")
	if err != nil {
		log.Fatal(err)
	}
	var path string
	var notFound []string

	for rows.Next() {
		err := rows.Scan(&path)
		if err != nil {
			log.Panic(err)
		}
		if _, err := os.Stat(glroot + path); errors.Is(err, os.ErrNotExist) {
			notFound = append(notFound, path)
		}

	}
	rows.Close()
	// Delete the results from the database
	if len(notFound) != 0 {
		for _, path := range notFound {
			delentry(path)
		}
	}
}

func predir(search string) {
	var rows *sql.Rows
	var err error
	if *c == true {
		rows, err = database.DBCon.Query("SELECT path FROM release WHERE name = ? LIMIT 1", search)
	} else {
		rows, err = database.DBCon.Query("SELECT path FROM release WHERE name LIKE ? ORDER BY path DESC LIMIT 1", search)
	}
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	if rows.Next() {
		os.Exit(2)
	} else {
		os.Exit(0)
	}
}

func filter(checkme string) bool {
	disc := regexp.MustCompile(`/disc[-_.]?[0-9][0-9]?`)
	discmatch := disc.MatchString(checkme)
	cd := regexp.MustCompile(`/cd[-_.]?[0-9][0-9]?`)
	cdmatch := cd.MatchString(checkme)
	dvd := regexp.MustCompile(`/dvd[-_.]?[0-9][0-9]?`)
	dvdmatch := dvd.MatchString(checkme)
	if strings.Contains(checkme, "/subs/") || strings.Contains(checkme, "/sub/") || strings.Contains(checkme, "/sample/") || strings.Contains(checkme, "/proof/") || strings.Contains(checkme, "/cover/") || strings.Contains(checkme, " complete ") || strings.Contains(checkme, " incomplete ") || strings.Contains(checkme, "imdb") || strings.Contains(checkme, "/_") || discmatch || cdmatch || dvdmatch {
		return true
	} else {
		return false
	}
}

func walkfilter(path string, di fs.DirEntry, err error) error {
	if di.IsDir() {
		checkme := strings.ToLower(path + "/")
		result := filter(checkme)
		if result == false {
			// Add to sqlite database here, making sure to check that we dont already have an entry for it, or if it moved
			glpath := strings.ReplaceAll(path, *G+"/site", "")
			addentry(glpath, di.Name())
			return nil
		} else {
			return filepath.SkipDir
		}
	}
	return nil
}

func scan(glroot string, path string) {
	err := filepath.WalkDir(glroot+path, walkfilter)
	if err != nil {
		fmt.Printf("error walking: %v\n", err)
		return
	}
}

func search(search string, limit int) {
	fmt.Printf("Searching for %s...\n\n", search)
	rows, err := database.DBCon.Query("SELECT path FROM release WHERE name LIKE ? ORDER BY path DESC", "%"+search+"%")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	var nResults int
	var sResults []string
	var path string

	nResults = 0
	for rows.Next() {
		nResults++
		err := rows.Scan(&path)
		if err != nil {
			log.Panic(err)
		}
		sResults = append(sResults, path)
	}
	if nResults != 0 {
		var rResults []string
		for i := range sResults {
			if i < limit {
				n := sResults[len(sResults)-1-i]
				rResults = append(rResults, n)
			}
		}
		for _, path := range rResults {
			fmt.Printf("%s\n", path)
		}
	}
	fmt.Printf("\n%v result(s) found with a limit of %v.\n", nResults, limit)
}

func add(path string, release string) {
	path = strings.ReplaceAll(path, "/site", "")
	checkme := strings.ToLower(path + "/" + release)
	result := filter(checkme)
	if result == false {
		addentry(path+"/"+release, release)
	}
}

func del(path string, release string) {
	path = strings.ReplaceAll(path, "/site", "")
	delentry(path + "/" + release)
}

func main() {
	var err error
	flag.Parse()
	database.DBCon, err = sql.Open("sqlite3", "file:"+*G+*D+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		log.Fatal(err)
	}
	defer database.DBCon.Close()

	// Create and initialize the database if it does not exist
	if _, err := os.Stat(*G + *D); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Could not find database at %s, creating...\n", *G+*D)
		sqlStmt := `
		CREATE TABLE release (path text, lower text, name text, UNIQUE(path));
		DELETE FROM release;
		`
		_, err := database.DBCon.Exec(sqlStmt)
		if err != nil {
			log.Printf("%q: %s\n", err, sqlStmt)
			return
		}
	}
	switch *M {
	case "clean":
		clean(*G + "/site")
	case "predir":
		predir(*s)
	case "scan":
		scan(*G+"/site", *P)
	case "search":
		search(*s, *L)
	case "add":
		add(*p, *n)
	case "delete":
		del(*p, *n)
	default:
		search(*s, *L)
	}
}
