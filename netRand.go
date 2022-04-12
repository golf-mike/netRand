package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

///// global vars
const REQUESTS int = 100
const URL string = "https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"
const DBNAME string = "netRand.db"
const WGMIN int = 1
const WGMAX int = 11
const NREPEAT int = 10

//// types
type timingResult struct {
	WaitgroupSize       int
	ConcurrentTimingsMs [REQUESTS]int64
	ConcurrentTotalMs   int64
	SequentialTimingsMs [REQUESTS]int64
	SequentialTotalMs   int64
}

//// main
func main() {
	db := setupDb()
	defer db.Close()
	for i := WGMIN; i < WGMAX; i++ {
		// waitgroup size range
		for j := 0; j < NREPEAT; j++ {
			// repeat for more data points
			timings := requestTimes(i)
			persistTimings(timings, db)
		}
	}

}

func requestTimes(waitgroupSize int) timingResult {
	// do NTIMES requests in go routines with waitgroupSize
	// do NTIMES requests sequentially

	timings_concurrent, total_concurrent := concurrentRequests(waitgroupSize)
	timings_sequential, total_sequential := sequentialRequests()

	return timingResult{
		WaitgroupSize:       waitgroupSize,
		ConcurrentTimingsMs: timings_concurrent,
		ConcurrentTotalMs:   total_concurrent,
		SequentialTimingsMs: timings_sequential,
		SequentialTotalMs:   total_sequential,
	}

}
func persistTimings(timings timingResult, db *sql.DB) {
	persistRun(timings, db)
	currentRunId := getCurrentRunId(db)
	persistConcurrentTimings(currentRunId, timings, db)
	persistSequentialTimings(currentRunId, timings, db)
}
func concurrentRequests(waitgroupSize int) ([REQUESTS]int64, int64) {
	start := time.Now()

	var wg sync.WaitGroup
	var timings [REQUESTS]int64
	ch := make(chan int64, REQUESTS)

	for i := range timings {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doGetChannel(URL, ch)
		}()
		// waitgroupsize is controlled using modulo
		// making sure experiment size is always NTIMES
		// independent of waitgroupsize
		if i%waitgroupSize == 0 {
			wg.Wait()
		}
	}
	wg.Wait()
	close(ch)

	count := 0
	for ret := range ch {
		timings[count] = ret
		count++
	}

	return timings, time.Since(start).Milliseconds()
}
func doGetChannel(address string, channel chan int64) {
	// time get request and send to channel
	startSub := time.Now().UnixMilli()
	_, err := http.Get(address)
	if err != nil {
		log.Fatalln(err)
	}
	stopSub := time.Now().UnixMilli()
	delta := stopSub - startSub
	channel <- delta
}
func sequentialRequests() ([REQUESTS]int64, int64) {
	startGo := time.Now()
	var timings_sequential [REQUESTS]int64
	for i := range timings_sequential {
		timings_sequential[i] = doGetReturn(URL)
	}
	return timings_sequential, time.Since(startGo).Milliseconds()
}
func doGetReturn(address string) int64 {
	// time get request without a waitgroup/channel
	start := time.Now()
	_, err := http.Get(address)
	if err != nil {
		log.Fatalln(err)
	}
	duration := time.Since(start).Milliseconds()
	return duration
}

//// DB
func setupDb() *sql.DB {
	//      __________________________runs____________________
	//     |                                                  |
	// concurrent_timings(fk: run_id)         sequential_timings(fk: run_id)
	//
	const createRuns string = `
    CREATE TABLE IF NOT EXISTS runs (
    run_id INTEGER NOT NULL PRIMARY KEY,
    time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    waitgroup_size INTEGER,
    concurrent_total_ms INTEGER,
    sequential_total_ms INTEGER,
    concurrent_sequential_ratio REAL
    );`

	const createSequentialTimings string = `
	CREATE TABLE IF NOT EXISTS sequential_timings (
	run INTEGER,
	call_number INTEGER,
	timing_ms INTEGER,
	FOREIGN KEY(run) REFERENCES runs(run_id)
	);`

	const createConcurrentTimings string = `
	CREATE TABLE IF NOT EXISTS concurrent_timings (
	run INTEGER,
	channel_position INTEGER,
	timing_ms INTEGER,
	FOREIGN KEY(run) REFERENCES runs(run_id)
	);`
	// retrieve platform appropriate connection string
	dbString := getConnectionString(DBNAME)
	db, err := sql.Open("sqlite3", dbString)
	if err != nil {
		log.Fatalln(err)
	}
	if _, err := db.Exec(createRuns); err != nil {
		log.Fatalln(err)
	}
	if _, err := db.Exec(createSequentialTimings); err != nil {
		log.Fatalln(err)
	}
	if _, err := db.Exec(createConcurrentTimings); err != nil {
		log.Fatalln(err)
	}
	return db
}
func getConnectionString(dbName string) string {
	// Generate platform appropriate connection string
	// the db is placed in the same directory as the current executable

	// retrieve the path to the currently executed executable
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	// retrieve path to containing dir
	dbDir := filepath.Dir(ex)

	// Append platform appropriate separator and dbName
	if runtime.GOOS == "windows" {

		dbDir = dbDir + "\\" + dbName

	} else {
		dbDir = dbDir + "/" + dbName
	}
	return dbDir
}
func persistRun(timings timingResult, db *sql.DB) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatalln(err)
	}

	insertRun, err := db.Prepare(`INSERT INTO runs(
		waitgroup_size, 
		sequential_total_ms, 
		concurrent_total_ms, 
		concurrent_sequential_ratio) 
		VALUES(?, ?, ?, ?)`)

	if err != nil {
		log.Fatalln(err)
	}
	defer tx.Stmt(insertRun).Close()
	_, err = tx.Stmt(insertRun).Exec(
		timings.WaitgroupSize,
		timings.SequentialTotalMs,
		timings.ConcurrentTotalMs,
		float32(timings.ConcurrentTotalMs)/float32(timings.SequentialTotalMs),
	)
	if err != nil {
		log.Fatalln(err)
	}
	err = tx.Commit()

	if err != nil {
		log.Fatalln(err)
	}
}

func getCurrentRunId(db *sql.DB) int {
	rows, err := db.Query("SELECT MAX(run_id) FROM runs")
	if err != nil {
		log.Fatal(err)
	}
	var run_id int
	for rows.Next() {
		err = rows.Scan(&run_id)
		if err != nil {
			log.Fatalln(err)
		}
	}
	rows.Close()
	return run_id
}
func persistConcurrentTimings(runId int, timings timingResult, db *sql.DB) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatalln(err)
	}

	insertTiming, err := db.Prepare(`INSERT INTO concurrent_timings(
		run, 
		channel_position, 
		timing_ms) 
		VALUES(?, ?, ?)`)

	if err != nil {
		log.Fatalln(err)
	}
	for i, timing := range timings.ConcurrentTimingsMs {
		_, err = tx.Stmt(insertTiming).Exec(
			runId,
			i,
			timing,
		)
		if err != nil {
			log.Fatalln(err)
		}
	}

	err = tx.Commit()

	if err != nil {
		log.Fatalln(err)
	}
}
func persistSequentialTimings(runId int, timings timingResult, db *sql.DB) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatalln(err)
	}

	insertTiming, err := db.Prepare(`INSERT INTO sequential_timings(
		run, 
		call_number, 
		timing_ms) 
		VALUES(?, ?, ?)`)

	if err != nil {
		log.Fatalln(err)
	}
	for i, timing := range timings.SequentialTimingsMs {
		_, err = tx.Stmt(insertTiming).Exec(
			runId,
			i,
			timing,
		)
		if err != nil {
			log.Fatalln(err)
		}
	}

	err = tx.Commit()

	if err != nil {
		log.Fatalln(err)
	}
}
