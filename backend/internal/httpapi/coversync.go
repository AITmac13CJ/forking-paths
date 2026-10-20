package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"forking-paths/internal/platform"
	"forking-paths/internal/store"
)

func (s *Server) saveGridKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, &store.ValidationError{Msg: "请求格式不对"})
		return
	}
	secret := strings.TrimSpace(body.Secret)
	if err := platform.NewClient().CheckGridKey(r.Context(), secret); err != nil {
		writeErr(w, &store.ValidationError{Msg: err.Error()})
		return
	}
	if err := s.store.SaveGridKey(r.Context(), secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) matchCovers(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.GridKey(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	job, err := s.store.StartJob(r.Context(), "covers")
	if err != nil {
		writeErr(w, err)
		return
	}
	uid, _ := store.UserID(r.Context())
	go s.performCovers(job.ID, uid)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) performCovers(jobID, uid int) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("封面同步中断: %v", rec)
			s.finishJob(uid, jobID, "error", "同步中断了", 0, 0)
		}
	}()
	work, cancel := context.WithTimeout(store.WithUser(context.Background(), uid), 45*time.Minute)
	defer cancel()
	report := func(done, total int, message string) {
		ctx, cancel := context.WithTimeout(store.WithUser(context.Background(), uid), 10*time.Second)
		defer cancel()
		_ = s.store.TouchJob(ctx, jobID, done, total, message)
	}
	matched, missed, err := s.matchCoverLibrary(work, report)
	if err != nil {
		s.finishJob(uid, jobID, "error", err.Error(), 0, 0)
		return
	}
	s.finishJob(uid, jobID, "done", fmt.Sprintf("封面配上 %d 条。没找到图的还是原来的底色。", matched), missed, matched)
}

func (s *Server) matchCoverLibrary(ctx context.Context, report func(done, total int, message string)) (matched, missed int, err error) {
	key, err := s.store.GridKey(ctx)
	if err != nil {
		return 0, 0, err
	}
	pending, err := s.store.PendingSteamCovers(ctx)
	if err != nil {
		return 0, 0, err
	}
	portraitIDs := make([]string, 0, len(pending))
	landscapeIDs := make([]string, 0, len(pending))
	for _, item := range pending {
		if item.AppID == "" {
			continue
		}
		if item.Portrait {
			portraitIDs = append(portraitIDs, item.AppID)
		}
		if item.Landscape {
			landscapeIDs = append(landscapeIDs, item.AppID)
		}
	}
	if report != nil {
		report(0, len(pending), "正在查找封面")
	}
	client := platform.NewClient()
	arts, err := client.GridArts(ctx, key, portraitIDs, landscapeIDs)
	if err != nil && len(arts) == 0 && len(portraitIDs)+len(landscapeIDs) > 0 {
		return 0, 0, err
	}
	foundGame := map[string]int{}
	for i := range pending {
		if report != nil {
			report(i+1, len(pending), "正在查找封面")
		}
		art := arts[pending[i].AppID]
		needPortrait := pending[i].Portrait && art.Portrait == ""
		needLandscape := pending[i].Landscape && art.Landscape == ""
		if !needPortrait && !needLandscape {
			continue
		}
		title := pending[i].Title
		gameID, ok := foundGame[title]
		if !ok && title != "" {
			gameID, err = client.FindGridGame(ctx, key, title)
			if err != nil {
				return 0, 0, err
			}
			foundGame[title] = gameID
		}
		if gameID <= 0 {
			continue
		}
		extra, err := client.GridGameArt(ctx, key, gameID, needPortrait, needLandscape)
		if err != nil {
			return 0, 0, err
		}
		if art.Portrait == "" {
			art.Portrait = extra.Portrait
		}
		if art.Landscape == "" {
			art.Landscape = extra.Landscape
		}
		if pending[i].AppID != "" {
			arts[pending[i].AppID] = art
		} else {
			arts[titleKey(pending[i].ID)] = art
		}
	}

	type coverJob struct {
		item store.SteamCover
		art  platform.GridSide
	}
	work := make([]coverJob, 0)
	skipped := 0
	for _, item := range pending {
		art := arts[item.AppID]
		if item.AppID == "" {
			art = arts[titleKey(item.ID)]
		}
		portraitMissing := item.Portrait && art.Portrait == ""
		landscapeMissing := item.Landscape && art.Landscape == ""
		if (!item.Portrait || portraitMissing) && (!item.Landscape || landscapeMissing) {
			skipped++
			continue
		}
		work = append(work, coverJob{item: item, art: art})
	}
	if report != nil {
		report(0, len(work), "正在记下封面")
	}
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	doneDownloads := 0
	for _, job := range work {
		wg.Add(1)
		sem <- struct{}{}
		go func(item store.SteamCover, art platform.GridSide) {
			defer wg.Done()
			defer func() { <-sem }()
			portrait, landscape := "", ""
			if item.Portrait && art.Portrait != "" {
				portrait = platform.GridURL(art.Portrait)
			}
			if item.Landscape && art.Landscape != "" {
				landscape = platform.GridURL(art.Landscape)
			}
			saved := portrait != "" || landscape != ""
			if saved {
				if err := s.store.FillSteamCover(ctx, item.ID, portrait, landscape); err != nil {
					saved = false
				}
			}
			mu.Lock()
			if saved {
				matched++
			} else {
				missed++
			}
			doneDownloads++
			current := doneDownloads
			mu.Unlock()
			if report != nil {
				report(current, len(work), "正在记下封面")
			}
		}(job.item, job.art)
	}
	wg.Wait()
	return matched, missed + skipped, nil
}

func titleKey(id int) string {
	return "title:" + strconv.Itoa(id)
}
