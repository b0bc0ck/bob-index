package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/karrick/godirwalk"
	_ "github.com/mattn/go-sqlite3"
)

var M = flag.String("M", "search", "Mode selection (clean, scan, search)")
var D = flag.String("D", "/ftp-data/bob/bob-index.db", "Location of database")
var G = flag.String("G", "/home/ftpd/glftpd", "gl root path")
var P = flag.String("P", "/mp3", "Scan path (inside glroot)")
var L = flag.Int("L", 50, "Limit number of search results")
var s = flag.String("s", "test", "search string")
var p = flag.String("p", "/private/", "path for individual add or delete")
var n = flag.String("n", "test", "name of release for individual add or delete")
var d = flag.Bool("d", false, "debug")

func addentry(db *sql.DB, path string, name string) {
	result, err := db.Exec("INSERT or IGNORE INTO release(path, lower, name) VALUES (?, ?, ?)", path, strings.ToLower(name), name)
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

func delentry(db *sql.DB, path string) {
	result, err := db.Exec("DELETE FROM release WHERE path = ?", path)
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

func clean(db *sql.DB, glroot string) {
	fmt.Printf("Cleaning up database at %v\n", *G+*D)
	// Compile a slice with all release paths that no longer exist
	rows, err := db.Query("SELECT path FROM release")
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
			delentry(db, path)
		}
	}
}

func predir(db *sql.DB, search string) {
	rows, err := db.Query("SELECT path FROM release WHERE name = ? LIMIT 1", search)
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

func scan(db *sql.DB, glroot string, path string) {
	err := godirwalk.Walk(glroot+path, &godirwalk.Options{
		Unsorted: true,
		Callback: func(osPathname string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				checkme := strings.ToLower(osPathname)
				if strings.Contains(checkme, "/subs") || strings.Contains(checkme, "/sample") || strings.Contains(checkme, "/proof") || strings.Contains(checkme, " complete ") || strings.Contains(checkme, " incomplete ") || strings.Contains(checkme, "imdb") || strings.Contains(checkme, "/cd") || strings.Contains(checkme, "/dvd") || strings.Contains(checkme, "/disc") {
					return godirwalk.SkipThis
				}
				// Add to sqlite database here, making sure to check that we dont already have an entry for it, or if it moved
				glpath := strings.ReplaceAll(osPathname, glroot, "")
				addentry(db, glpath, string(de.Name()))
			}
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func search(db *sql.DB, search string, limit int) {
	fmt.Printf("Searching for %s...\n\n", search)
	rows, err := db.Query("SELECT path FROM release WHERE name LIKE ? ORDER BY path DESC LIMIT ?", "%"+search+"%", limit)
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
			n := sResults[len(sResults)-1-i]
			rResults = append(rResults, n)
		}
		for _, path := range rResults {
			fmt.Printf("%s\n", path)
		}
	}
	fmt.Printf("\n%v result(s) found with a limit of %v.\n", nResults, limit)
}

func add(db *sql.DB, path string, release string) {
	path = strings.ReplaceAll(path, "/site", "")
	checkme := strings.ToLower(path + "/" + release)
	if strings.Contains(checkme, "/subs") || strings.Contains(checkme, "/sample") || strings.Contains(checkme, "/proof") || strings.Contains(checkme, "/cd") || strings.Contains(checkme, "/dvd") || strings.Contains(checkme, "/disc") {
	} else {
		addentry(db, path+"/"+release, release)
	}
}

func del(db *sql.DB, path string, release string) {
	path = strings.ReplaceAll(path, "/site", "")
	delentry(db, path+"/"+release)
}

func main() {
	flag.Parse()
	db, err := sql.Open("sqlite3", "file:"+*G+*D+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create and initialize the database if it does not exist
	if _, err := os.Stat(*G + *D); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Could not find database at %s, creating...\n", *G+*D)
		sqlStmt := `
		CREATE TABLE release (path text, lower text, name text, UNIQUE(path));
		DELETE FROM release;
		`
		_, err := db.Exec(sqlStmt)
		if err != nil {
			log.Printf("%q: %s\n", err, sqlStmt)
			return
		}
	}
	switch *M {
	case "clean":
		clean(db, *G+"/site")
	case "predir":
		predir(db, *s)
	case "scan":
		scan(db, *G+"/site", *P)
	case "search":
		search(db, *s, *L)
	case "add":
		add(db, *p, *n)
	case "delete":
		del(db, *p, *n)
	default:
		search(db, *s, *L)
	}
}
