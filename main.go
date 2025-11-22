package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// --- STRUKTUREN ---

type Series struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	Year            string `json:"year"`
	IMDBID          string `json:"imdb_id"`
	EpisodesWatched int    `json:"episodes_watched"`
	TotalEpisodes   int    `json:"total_episodes"`
	Status          string `json:"status"`
	Progress        int    `json:"progress"`
	CoverURL        string `json:"cover_url"`
}

type OMDbResponse struct {
	Title        string `json:"Title"`
	Year         string `json:"Year"`
	TotalSeasons string `json:"totalSeasons"`
	IMDBID       string `json:"imdbID"`
	Response     string `json:"Response"`
	Error        string `json:"Error"`
	Poster       string `json:"Poster"`
}

type SearchResult struct {
	Search       []SearchItem `json:"Search"`
	Response     string       `json:"Response"`
	Error        string       `json:"Error"`
	TotalResults string       `json:"totalResults"`
}

type SearchItem struct {
	Title  string `json:"Title"`
	Year   string `json:"Year"`
	IMDBID string `json:"imdbID"`
	Type   string `json:"Type"`
	Poster string `json:"Poster"`
}

type User struct {
	DisplayName string `json:"display_name"`
	Theme       string `json:"theme"`
	Lang        string `json:"lang"`
	IsAdmin     bool   `json:"is_admin"`
}

type PageData struct {
	SeriesList      []Series
	SearchResults   []SearchItem
	SearchQuery     string
	ErrorMessage    string
	SuccessMessage  string
	APIAvailable    bool
	TotalSeries     int
	TotalWatched    int
	SortBy          string
	Order           string
	CurrentUser     string
	CurrentUserName string // ← Wird im Template als "Angemeldet als: ..." angezeigt
	IsAdmin         bool
}

// --- GLOBALE VARIABLEN ---

var (
	apiKey = os.Getenv("OMDb_API_KEY")

	templates *template.Template
	mutex     sync.Mutex

	users = map[string]User{
		"user_a": {DisplayName: "Nutzer A", Theme: "dark", Lang: "de", IsAdmin: true},
		"user_b": {DisplayName: "Nutzer B", Theme: "light", Lang: "de", IsAdmin: false},
		"user_c": {DisplayName: "Nutzer C", Theme: "light", Lang: "de", IsAdmin: false},
		"user_d": {DisplayName: "Nutzer D", Theme: "light", Lang: "de", IsAdmin: false},
	}

	httpClient = &http.Client{
		Timeout: 15 * time.Second,
	}
)

// --- HILFSFUNKTIONEN ---

func getDataFileForUser(username string) string {
	return filepath.Join("data", username+".json")
}

func getUsersFile() string {
	return filepath.Join("data", "users.json")
}

func loadUsers() {
	file := getUsersFile()
	if _, err := os.Stat(file); os.IsNotExist(err) {
		saveUsers()
		return
	}
	data, err := os.ReadFile(file)
	if err != nil {
		log.Printf("failed to load users.json: %v", err)
		return
	}
	var loadedUsers map[string]User
	if err := json.Unmarshal(data, &loadedUsers); err != nil {
		log.Printf("failed to parse users.json: %v", err)
		return
	}
	for k, v := range loadedUsers {
		if _, exists := users[k]; exists {
			users[k] = v
		}
	}
}

func saveUsers() {
	mutex.Lock()
	defer mutex.Unlock()

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		log.Printf("failed to marshal users: %v", err)
		return
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Printf("failed to create data dir: %v", err)
		return
	}
	if err := os.WriteFile(getUsersFile(), data, 0644); err != nil {
		log.Printf("failed to write users.json: %v", err)
	}
}

func loadSeriesForUser(username string) []Series {
	file := getDataFileForUser(username)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return []Series{}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		log.Printf("failed to read %s: %v", file, err)
		return []Series{}
	}
	var series []Series
	if err := json.Unmarshal(data, &series); err != nil {
		log.Printf("failed to unmarshal %s: %v", file, err)
		return []Series{}
	}
	return series
}

func saveSeriesForUser(username string, series []Series) {
	mutex.Lock()
	defer mutex.Unlock()

	data, err := json.MarshalIndent(series, "", "  ")
	if err != nil {
		log.Printf("failed to marshal series for %s: %v", username, err)
		return
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Printf("failed to create data dir: %v", err)
		return
	}
	if err := os.WriteFile(getDataFileForUser(username), data, 0644); err != nil {
		log.Printf("failed to write %s: %v", getDataFileForUser(username), err)
	}
}

func getCurrentUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("user")
	if err != nil {
		return "", false
	}
	if _, exists := users[cookie.Value]; exists {
		return cookie.Value, true
	}
	return "", false
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := getCurrentUser(r); ok {
			next(w, r)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	}
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := getCurrentUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !users[user].IsAdmin {
			data := PageData{
				ErrorMessage:    "Zugriff verweigert: Nur für Administratoren",
				CurrentUser:     user,
				CurrentUserName: users[user].DisplayName,
				IsAdmin:         false,
			}
			w.WriteHeader(http.StatusForbidden)
			templates.ExecuteTemplate(w, "index.html", data)
			return
		}
		next(w, r)
	}
}

// --- HANDLER ---

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		user := r.FormValue("user")
		valid := map[string]bool{
			"user_a": true,
			"user_b": true,
			"user_c": true,
			"user_d": true,
		}
		if !valid[user] {
			http.Error(w, "invalid user", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:  "user",
			Value: user,
			Path:  "/",
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	templates.ExecuteTemplate(w, "login.html", nil)
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := getCurrentUser(r)
	if r.Method == "POST" {
		if name := r.FormValue("user_b_name"); name != "" {
			u := users["user_b"]
			u.DisplayName = name
			users["user_b"] = u
		}
		if name := r.FormValue("user_c_name"); name != "" {
			u := users["user_c"]
			u.DisplayName = name
			users["user_c"] = u
		}
		if name := r.FormValue("user_d_name"); name != "" {
			u := users["user_d"]
			u.DisplayName = name
			users["user_d"] = u
		}
		saveUsers()

		delUser := r.FormValue("delete_user")
		if delUser != "" && delUser != "user_a" {
			if _, exists := users[delUser]; exists {
				os.Remove(getDataFileForUser(delUser))
			}
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	data := PageData{
		CurrentUser:     user,
		CurrentUserName: users[user].DisplayName,
		IsAdmin:         true,
	}
	templates.ExecuteTemplate(w, "admin.html", data)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	series := loadSeriesForUser(user)
	totalSeries, totalWatched := calculateStats(series)
	apiAvailable := testAPIConnection()

	data := PageData{
		SeriesList:      series,
		APIAvailable:    apiAvailable,
		TotalSeries:     totalSeries,
		TotalWatched:    totalWatched,
		CurrentUser:     user,
		CurrentUserName: users[user].DisplayName,
		IsAdmin:         users[user].IsAdmin,
	}
	templates.ExecuteTemplate(w, "index.html", data)
}

func myListHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	series := loadSeriesForUser(user)
	sortParam := r.URL.Query().Get("sort")
	var sortBy, order string
	switch sortParam {
	case "title":
		sortBy, order = "title", "asc"
	case "title_desc":
		sortBy, order = "title", "desc"
	case "progress_asc":
		sortBy, order = "progress", "asc"
	case "progress_desc":
		sortBy, order = "progress", "desc"
	default:
		sortBy, order = "title", "asc"
	}
	sortSeries(series, sortBy, order)

	totalSeries := len(series)
	totalEpisodesWatched := 0
	for _, s := range series {
		totalEpisodesWatched += s.EpisodesWatched
	}

	data := PageData{
		SeriesList:      series,
		APIAvailable:    testAPIConnection(),
		TotalSeries:     totalSeries,
		TotalWatched:    totalEpisodesWatched,
		SortBy:          sortBy,
		Order:           order,
		CurrentUser:     user,
		CurrentUserName: users[user].DisplayName,
		IsAdmin:         users[user].IsAdmin,
	}
	templates.ExecuteTemplate(w, "mylist.html", data)
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")
	if identifier == "" {
		http.Error(w, "identifier required", http.StatusBadRequest)
		return
	}

	seriesData, err := fetchIMDBData(identifier)
	if err != nil {
		seriesList := loadSeriesForUser(user)
		totalSeries, totalWatched := calculateStats(seriesList)
		data := PageData{
			SeriesList:      seriesList,
			ErrorMessage:    fmt.Sprintf("failed to add series: %v", err),
			APIAvailable:    testAPIConnection(),
			TotalSeries:     totalSeries,
			TotalWatched:    totalWatched,
			CurrentUser:     user,
			CurrentUserName: users[user].DisplayName,
			IsAdmin:         users[user].IsAdmin,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	totalEpisodes := 0
	if seriesData.TotalSeasons != "" {
		if seasons, err := strconv.Atoi(seriesData.TotalSeasons); err == nil {
			totalEpisodes = seasons * 10
		}
	}

	seriesDB := loadSeriesForUser(user)
	for _, s := range seriesDB {
		if s.IMDBID == seriesData.IMDBID {
			seriesList := loadSeriesForUser(user)
			totalSeries, totalWatched := calculateStats(seriesList)
			data := PageData{
				SeriesList:      seriesList,
				ErrorMessage:    "series already in your library",
				APIAvailable:    testAPIConnection(),
				TotalSeries:     totalSeries,
				TotalWatched:    totalWatched,
				CurrentUser:     user,
				CurrentUserName: users[user].DisplayName,
				IsAdmin:         users[user].IsAdmin,
			}
			templates.ExecuteTemplate(w, "index.html", data)
			return
		}
	}

	nextID := 1
	for _, s := range seriesDB {
		if s.ID >= nextID {
			nextID = s.ID + 1
		}
	}

	newSeries := Series{
		ID:            nextID,
		Title:         seriesData.Title,
		Year:          seriesData.Year,
		IMDBID:        seriesData.IMDBID,
		TotalEpisodes: totalEpisodes,
		Status:        "Watching",
		CoverURL:      seriesData.Poster,
	}

	seriesDB = append(seriesDB, newSeries)
	saveSeriesForUser(user, seriesDB)

	seriesList := loadSeriesForUser(user)
	totalSeries, totalWatched := calculateStats(seriesList)
	data := PageData{
		SeriesList:      seriesList,
		SuccessMessage:  fmt.Sprintf("✅ '%s' added successfully!", seriesData.Title),
		APIAvailable:    testAPIConnection(),
		TotalSeries:     totalSeries,
		TotalWatched:    totalWatched,
		CurrentUser:     user,
		CurrentUserName: users[user].DisplayName,
		IsAdmin:         users[user].IsAdmin,
	}
	templates.ExecuteTemplate(w, "index.html", data)
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	episodesStr := r.FormValue("episodes")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	episodes, err := strconv.Atoi(episodesStr)
	if err != nil {
		http.Error(w, "invalid episodes number", http.StatusBadRequest)
		return
	}

	seriesDB := loadSeriesForUser(user)
	found := false
	for i := range seriesDB {
		if seriesDB[i].ID == id {
			seriesDB[i].EpisodesWatched = episodes
			found = true
			break
		}
	}
	if found {
		saveSeriesForUser(user, seriesDB)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	seriesDB := loadSeriesForUser(user)
	newSeries := []Series{}
	for _, s := range seriesDB {
		if s.ID != id {
			newSeries = append(newSeries, s)
		}
	}
	saveSeriesForUser(user, newSeries)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	results, err := searchIMDBData(query)
	if err != nil {
		seriesList := loadSeriesForUser(user)
		totalSeries, totalWatched := calculateStats(seriesList)
		data := PageData{
			SeriesList:      seriesList,
			SearchQuery:     query,
			ErrorMessage:    fmt.Sprintf("search failed: %v", err),
			APIAvailable:    testAPIConnection(),
			TotalSeries:     totalSeries,
			TotalWatched:    totalWatched,
			CurrentUser:     user,
			CurrentUserName: users[user].DisplayName,
			IsAdmin:         users[user].IsAdmin,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	var seriesResults []SearchItem
	for _, item := range results.Search {
		if item.Type == "series" {
			seriesResults = append(seriesResults, item)
		}
	}

	seriesList := loadSeriesForUser(user)
	totalSeries, totalWatched := calculateStats(seriesList)
	data := PageData{
		SeriesList:      seriesList,
		SearchResults:   seriesResults,
		SearchQuery:     query,
		APIAvailable:    testAPIConnection(),
		TotalSeries:     totalSeries,
		TotalWatched:    totalWatched,
		CurrentUser:     user,
		CurrentUserName: users[user].DisplayName,
		IsAdmin:         users[user].IsAdmin,
	}

	if len(seriesResults) == 0 && len(results.Search) > 0 {
		data.ErrorMessage = "no series found (only movies or other types)"
	} else if len(seriesResults) == 0 {
		data.ErrorMessage = "no results found"
	}

	templates.ExecuteTemplate(w, "index.html", data)
}

func pdfHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	series := loadSeriesForUser(user)
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	utf8 := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 20)
	pdf.Cell(0, 10, utf8("Meine Serienliste"))
	pdf.Ln(15)

	countOnPage := 0
	for _, s := range series {
		if countOnPage == 4 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "B", 20)
			pdf.Cell(0, 10, utf8("Meine Serienliste (Fortsetzung)"))
			pdf.Ln(15)
			countOnPage = 0
		}

		imgWidth := 40.0
		startY := pdf.GetY()
		var imgHeight float64 = 20

		if s.CoverURL != "" && s.CoverURL != "N/A" {
			resp, err := httpClient.Get(s.CoverURL)
			if err == nil {
				func() {
					defer resp.Body.Close()
					data, err := io.ReadAll(resp.Body)
					if err != nil {
						return
					}
					imgName := fmt.Sprintf("cover_%d", s.ID)
					info := pdf.RegisterImageOptionsReader(
						imgName,
						gofpdf.ImageOptions{ImageType: "JPG", ReadDpi: true},
						bytes.NewReader(data),
					)
					if info != nil && info.Width() > 0 {
						imgHeight = info.Height() * imgWidth / info.Width()
						pdf.ImageOptions(
							imgName, 10, startY, imgWidth, 0,
							false,
							gofpdf.ImageOptions{ImageType: "JPG", ReadDpi: true},
							0, "",
						)
					}
				}()
			}
		}

		textX := 10 + imgWidth + 6
		pdf.SetXY(textX, startY)
		pdf.SetFont("Helvetica", "B", 14)
		pdf.CellFormat(0, 7, utf8(fmt.Sprintf("%s (%s)", s.Title, s.Year)), "", 0, "L", false, 0, "")
		pdf.Ln(8)

		pdf.SetX(textX)
		pdf.SetFont("Helvetica", "", 12)
		pdf.MultiCell(0, 6,
			utf8(fmt.Sprintf("Status: %s – %d/%d Episoden", s.Status, s.EpisodesWatched, s.TotalEpisodes)),
			"", "L", false,
		)

		endY := pdf.GetY()
		finalY := startY + imgHeight
		if endY > finalY {
			finalY = endY
		}
		pdf.SetY(finalY + 10)
		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
		pdf.Ln(8)
		countOnPage++
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=mylist.pdf")
	if err := pdf.Output(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func apiSeriesHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getCurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	series := loadSeriesForUser(user)
	json.NewEncoder(w).Encode(series)
}

// --- HILFSFUNKTIONEN (SORT, STATS, API) ---

func sortSeries(series []Series, sortBy, order string) {
	switch sortBy {
	case "title":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				return series[i].Title > series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				return series[i].Title < series[j].Title
			})
		}
	case "progress":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				if series[i].Progress != series[j].Progress {
					return series[i].Progress > series[j].Progress
				}
				return series[i].Title < series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				if series[i].Progress != series[j].Progress {
					return series[i].Progress < series[j].Progress
				}
				return series[i].Title < series[j].Title
			})
		}
	case "watched":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				if series[i].EpisodesWatched != series[j].EpisodesWatched {
					return series[i].EpisodesWatched > series[j].EpisodesWatched
				}
				return series[i].Title < series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				if series[i].EpisodesWatched != series[j].EpisodesWatched {
					return series[i].EpisodesWatched < series[j].EpisodesWatched
				}
				return series[i].Title < series[j].Title
			})
		}
	default:
		sort.Slice(series, func(i, j int) bool {
			return series[i].Title < series[j].Title
		})
	}
}

func calculateStats(series []Series) (int, int) {
	totalSeries := len(series)
	totalCompleted := 0
	for _, s := range series {
		if s.Progress == 100 {
			totalCompleted++
		}
	}
	return totalSeries, totalCompleted
}

func testAPIConnection() bool {
	if apiKey == "" {
		return false
	}
	testURL := fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&t=Game%%20of%%20Thrones&r=json", apiKey)
	resp, err := httpClient.Get(testURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	var result struct {
		Response string `json:"Response"`
		Error    string `json:"Error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	return result.Response != "False"
}

func fetchIMDBData(identifier string) (*OMDbResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("omdb api key not set")
	}
	baseURL := "http://www.omdbapi.com/"
	params := url.Values{}
	params.Add("apikey", apiKey)
	params.Add("r", "json")
	if len(identifier) > 2 && identifier[:2] == "tt" {
		params.Add("i", identifier)
	} else {
		params.Add("t", url.QueryEscape(identifier))
		params.Add("type", "series")
	}
	urlStr := baseURL + "?" + params.Encode()
	resp, err := httpClient.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid api key (status 401)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api responded with status: %d", resp.StatusCode)
	}
	var result OMDbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}
	if result.Response == "False" {
		if result.Error != "" {
			return nil, fmt.Errorf("api error: %s", result.Error)
		}
		return nil, fmt.Errorf("series not found")
	}
	return &result, nil
}

func searchIMDBData(query string) (*SearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("omdb api key not set")
	}
	baseURL := "http://www.omdbapi.com/"
	params := url.Values{}
	params.Add("apikey", apiKey)
	params.Add("s", url.QueryEscape(query))
	params.Add("type", "series")
	params.Add("r", "json")
	params.Add("page", "1")
	urlStr := baseURL + "?" + params.Encode()
	resp, err := httpClient.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid api key (status 401)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api responded with status: %d", resp.StatusCode)
	}
	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}
	if result.Response == "False" {
		if result.Error != "" {
			return nil, fmt.Errorf("api error: %s", result.Error)
		}
		return nil, fmt.Errorf("no results found")
	}
	return &result, nil
}

// --- MAIN ---

func main() {
	if apiKey == "" {
		log.Println("⚠️  warning: OMDb_API_KEY environment variable not set")
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatal("failed to create data directory:", err)
	}
	loadUsers()

	templates = template.Must(template.ParseGlob("templates/*.html"))

	// WICHTIG: /login ohne Auth-Middleware!
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/admin", requireAdmin(adminHandler))
	http.HandleFunc("/", authMiddleware(indexHandler))
	http.HandleFunc("/mylist", authMiddleware(myListHandler))
	http.HandleFunc("/add", authMiddleware(addHandler))
	http.HandleFunc("/update", authMiddleware(updateHandler))
	http.HandleFunc("/delete", authMiddleware(deleteHandler))
	http.HandleFunc("/search", authMiddleware(searchHandler))
	http.HandleFunc("/api/series", authMiddleware(apiSeriesHandler))
	http.HandleFunc("/pdf", authMiddleware(pdfHandler))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	port := findAvailablePort()
	if port == 0 {
		port = 8080
	}

	fmt.Printf("🚀 serien-tracker running on http://localhost:%d\n", port)
	fmt.Printf("👉 go to http://localhost:%d/login\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func findAvailablePort() int {
	for port := 8080; port <= 8090; port++ {
		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port
		}
	}
	return 0
}
