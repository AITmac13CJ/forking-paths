package store

import "errors"

var ErrNotFound = errors.New("没有这条记录")
var ErrUnauthorized = errors.New("请先登录")

type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }

type Path struct {
	ID              int      `json:"id"`
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	MediumLabel     string   `json:"mediumLabel"`
	ViaLabel        string   `json:"viaLabel"`
	ReleaseLabel    string   `json:"releaseLabel"`
	SpendLabel      string   `json:"spendLabel"`
	FinishTimeLabel string   `json:"finishTimeLabel"`
	FinishDateLabel string   `json:"finishDateLabel"`
	DoneWord        string   `json:"doneWord"`
	DoingTitle      string   `json:"doingTitle"`
	StatusWant      string   `json:"statusWant"`
	StatusDoing     string   `json:"statusDoing"`
	StatusDone      string   `json:"statusDone"`
	StatusHold      string   `json:"statusHold"`
	Badges          []string `json:"badges"`
}

type Medium struct {
	ID     int    `json:"id"`
	PathID int    `json:"pathId"`
	Name   string `json:"name"`
	Color  string `json:"color"`
}

type Via struct {
	ID     int    `json:"id"`
	PathID int    `json:"pathId"`
	Name   string `json:"name"`
}

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Cover struct {
	A     string `json:"a"`
	B     string `json:"b"`
	Motif string `json:"motif"`
}

type Entry struct {
	ID                int      `json:"id"`
	Title             string   `json:"title"`
	Path              Path     `json:"path"`
	Medium            *Medium  `json:"medium"`
	Via               *Via     `json:"via"`
	Tags              []Tag    `json:"tags"`
	ReleaseDate       string   `json:"releaseDate"`
	Status            string   `json:"status"`
	SpendMinutes      int      `json:"spendMinutes"`
	FinishMinutes     *int     `json:"finishMinutes"`
	FinishDate        string   `json:"finishDate"`
	Rating            *float64 `json:"rating"`
	Badges            []string `json:"badges"`
	ProgressEarned    *int     `json:"progressEarned"`
	ProgressTotal     *int     `json:"progressTotal"`
	Cover             Cover    `json:"cover"`
	CoverURL          string   `json:"coverUrl"`
	CoverLandscapeURL string   `json:"coverLandscapeUrl"`
	Notes             string   `json:"notes"`
}

type EntryInput struct {
	Title             string   `json:"title"`
	PathID            int      `json:"pathId"`
	MediumID          int      `json:"mediumId"`
	ViaID             *int     `json:"viaId"`
	TagIDs            []int    `json:"tagIds"`
	ReleaseDate       string   `json:"releaseDate"`
	Status            string   `json:"status"`
	SpendMinutes      int      `json:"spendMinutes"`
	FinishMinutes     *int     `json:"finishMinutes"`
	FinishDate        string   `json:"finishDate"`
	Rating            *float64 `json:"rating"`
	Badges            []string `json:"badges"`
	ProgressEarned    *int     `json:"progressEarned"`
	ProgressTotal     *int     `json:"progressTotal"`
	Cover             Cover    `json:"cover"`
	CoverURL          string   `json:"coverUrl"`
	CoverLandscapeURL string   `json:"coverLandscapeUrl"`
	Notes             string   `json:"notes"`
}

type Sort struct {
	Field string
	Dir   string
}

type Filter struct {
	PathID    int
	Q         string
	Status    string
	Release   string
	Finish    string
	MediumIDs []int
	TagIDs    []int
	Sorts     []Sort
}

type Meta struct {
	Paths  []Path   `json:"paths"`
	Media  []Medium `json:"media"`
	Vias   []Via    `json:"vias"`
	Tags   []Tag    `json:"tags"`
	Motifs []string `json:"motifs"`
}

type MediumCount struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MonthCount struct {
	Month   int `json:"month"`
	Count   int `json:"count"`
	Minutes int `json:"minutes"`
}

type DayMark struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type Highlight struct {
	Title      string   `json:"title"`
	CoverURL   string   `json:"coverUrl"`
	Color      string   `json:"color"`
	Medium     string   `json:"medium"`
	Path       string   `json:"path"`
	Minutes    int      `json:"minutes"`
	Rating     *float64 `json:"rating"`
	FinishDate string   `json:"finishDate"`
}

type RankItem struct {
	Title    string   `json:"title"`
	CoverURL string   `json:"coverUrl"`
	Color    string   `json:"color"`
	Medium   string   `json:"medium"`
	Minutes  int      `json:"minutes"`
	Rating   *float64 `json:"rating"`
}

type PathShare struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Color   string `json:"color"`
	Count   int    `json:"count"`
	Minutes int    `json:"minutes"`
}

type Recent struct {
	Title      string   `json:"title"`
	FinishDate string   `json:"finishDate"`
	Medium     string   `json:"medium"`
	Path       string   `json:"path"`
	Rating     *float64 `json:"rating"`
}

type Stats struct {
	Year          int           `json:"year"`
	Done          int           `json:"done"`
	Doing         int           `json:"doing"`
	Want          int           `json:"want"`
	SpendMinutes  int           `json:"spendMinutes"`
	FinishMinutes int           `json:"finishMinutes"`
	AvgRating     *float64      `json:"avgRating"`
	Platinum      int           `json:"platinum"`
	Marked        int           `json:"marked"`
	ByMedium      []MediumCount `json:"byMedium"`
	ByMonth       []MonthCount  `json:"byMonth"`
	TopTags       []NameCount   `json:"topTags"`
	Recent        []Recent      `json:"recent"`
	ActiveDays    int           `json:"activeDays"`
	PrevDone      int           `json:"prevDone"`
	PrevSpend     int           `json:"prevSpend"`
	AvgSpend      int           `json:"avgSpend"`
	Days          []DayMark     `json:"days"`
	Longest       *Highlight    `json:"longest"`
	Favorite      *Highlight    `json:"favorite"`
	Ranking       []RankItem    `json:"ranking"`
	ByPath        []PathShare   `json:"byPath"`
	ByBadge       []NameCount   `json:"byBadge"`
	ByVia         []NameCount   `json:"byVia"`
}
