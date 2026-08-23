package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/P-PrPas/CT-Flow/backend/internal/core/metrics"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/vpe"
)

// Long inference passes over a folder run as a background job: the request
// returns a job_id immediately and the UI polls GET /api/jobs/{id}. Only
// `result`'s shape differs per job, which is why each endpoint documents its own
// -- GET /api/jobs/{id} cannot know which kind it is looking at.
//
// The inference happens in the sidecar, which streams one line per image back.
// This file owns the tracker, the database writes and the metrics, so every
// result shape is unchanged even though nothing here loads a model.

const emptyBank = "prompt bank is empty -- label something first"

// jobContext is what a background pass runs under.
//
// Deliberately NOT the request's context: that is cancelled the moment the
// response is written, which would kill every job the instant it started. The
// timeout is generous because a full pool pass over hundreds of images is
// legitimately long, but it is bounded so a wedged sidecar cannot leak a
// goroutine for the life of the process.
func jobContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 6*time.Hour)
}

// GetJob is the poll target for every background job. 404 once a job_id was
// never issued -- jobs are never pruned otherwise, so this only fires for a
// typo'd id or one from before a restart.
func (s *Server) GetJob(w http.ResponseWriter, r *http.Request) error {
	job, ok := s.Jobs.Get(r.PathValue("id"))
	if !ok {
		return errStatus(http.StatusNotFound, "unknown job")
	}
	// `now` from the server's clock, so the progress bar's ETA is immune to
	// browser clock skew.
	writeJSON(w, http.StatusOK, map[string]any{
		"done": job.Done, "total": job.Total, "started_at": job.StartedAt,
		"finished": job.Finished, "result": job.Result, "error": job.Error,
		"now": float64(time.Now().UnixNano()) / 1e9,
	})
	return nil
}

// requireBank rejects a job that cannot mean anything yet. One cheap call here
// beats starting a job that immediately fails, which the UI would show as a
// progress bar that dies.
func (s *Server) requireBank(r *http.Request, stateDir string) error {
	bank, err := s.VPE.Bank(r.Context(), stateDir)
	if err != nil {
		return err
	}
	if len(bank.Classes) == 0 {
		return errStatus(http.StatusBadRequest, emptyBank)
	}
	return nil
}

type jobRequest struct {
	InputDir    string             `json:"input_dir"`
	Images      []string           `json:"images"`
	Conf        *float64           `json:"conf"`
	ConfByClass map[string]float64 `json:"conf_by_class"`
	ModelID     string             `json:"model_id"`
}

// Score rescores the pool against the current bank, so the UI can order images
// by "where is the model least sure".
//
// Job result: {"scores": {image_path: {"conf": float, "cls": str|null, "sig": [int, ...]}}}
func (s *Server) Score(w http.ResponseWriter, r *http.Request) error {
	var req jobRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	_, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	paths, err := s.checkedPaths(req.Images)
	if err != nil {
		return err
	}
	bank, err := s.VPE.Bank(r.Context(), stateDir)
	if err != nil {
		return err
	}
	jobID := s.Jobs.Create(len(paths))
	if len(bank.Classes) == 0 || len(paths) == 0 {
		// Nothing to do is a finished job, not an error: the UI polls it once
		// and moves on.
		s.Jobs.Finish(jobID, map[string]any{"scores": map[string]any{}})
		writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "total": 0})
		return nil
	}

	go func() {
		ctx, cancel := jobContext()
		defer cancel()
		scores := map[string]any{}
		done := 0
		// conf 0.05 rather than the usual default: this pass ranks images by
		// "how sure is the model here", so a box below the labelling threshold
		// is still a signal worth recording.
		err := s.VPE.PredictStream(ctx, stateDir, paths, 0.05, nil, true,
			func(line vpe.StreamLine) error {
				var best *vpe.Detection
				for i := range line.Boxes {
					if best == nil || line.Boxes[i].Conf > best.Conf {
						best = &line.Boxes[i]
					}
				}
				entry := map[string]any{"conf": 0.0, "cls": nil, "sig": line.Sig}
				if best != nil {
					entry["conf"] = best.Conf
					entry["cls"] = best.Cls
				}
				scores[line.Image] = entry
				done++
				s.Jobs.Tick(jobID, done)
				return nil
			})
		if err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		s.Jobs.Finish(jobID, map[string]any{"scores": scores})
	}()

	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "total": len(paths)})
	return nil
}

// Evaluate measures the current bank against the held-out, hand-labeled test
// set. This is the readiness signal -- pool confidence only says which image to
// label next.
//
// Job result: overall/per_class precision-recall-F1 plus per_image detail.
func (s *Server) Evaluate(w http.ResponseWriter, r *http.Request) error {
	var req jobRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	if err := s.requireBank(r, stateDir); err != nil {
		return err
	}

	names, err := s.Store.Classes(r.Context(), inputDir, store.KindTestset)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return errStatus(http.StatusBadRequest,
			"no test-set ground truth for "+inputDir+" -- label the test set first")
	}
	all, err := s.Store.LoadAnnotations(r.Context(), inputDir, store.KindTestset)
	if err != nil {
		return err
	}
	// An image annotated once and since deleted has a row but nothing to
	// predict on; measuring against it would count guaranteed misses.
	gt := map[string][]store.Box{}
	for path, boxes := range all {
		if len(boxes) > 0 && isFile(path) {
			gt[path] = boxes
		}
	}
	if len(gt) == 0 {
		return errStatus(http.StatusBadRequest, "no labeled images found in test set for "+inputDir)
	}
	order := make([]string, 0, len(gt))
	for p := range gt {
		order = append(order, p)
	}
	sortStrings(order)

	conf := orDefaultConf(req.Conf)
	confByClass := req.ConfByClass
	jobID := s.Jobs.Create(len(gt))

	go func() {
		ctx, cancel := jobContext()
		defer cancel()
		pred := map[string][]vpe.Detection{}
		done := 0
		err := s.VPE.PredictStream(ctx, stateDir, order, conf, confByClass, false,
			func(line vpe.StreamLine) error {
				pred[line.Image] = line.Boxes
				done++
				s.Jobs.Tick(jobID, done)
				return nil
			})
		if err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		result := metrics.Evaluate(gt, order, pred, metrics.DefaultIoU)
		// The thresholds are echoed back with the numbers, so a recorded
		// history point says what it was measured at.
		s.Jobs.Finish(jobID, withThresholds(result, conf, confByClass))
	}()

	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "total": len(gt)})
	return nil
}

// Autolabel writes labels for these images straight from the bank. Only worth
// doing once the test-set numbers say the bank is good enough.
//
// Job result: {"written": int, "no_detection": int, "no_detection_images": [str], "bank": dict}
func (s *Server) Autolabel(w http.ResponseWriter, r *http.Request) error {
	var req jobRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	if err := s.requireBank(r, stateDir); err != nil {
		return err
	}
	paths, err := s.checkedPaths(req.Images)
	if err != nil {
		return err
	}
	conf := orDefaultConf(req.Conf)
	confByClass := req.ConfByClass
	jobID := s.Jobs.Create(len(paths))

	go func() {
		ctx, cancel := jobContext()
		defer cancel()
		written := 0
		empty := []string{}
		autoPaths := []string{}
		done := 0
		err := s.VPE.PredictStream(ctx, stateDir, paths, conf, confByClass, false,
			func(line vpe.StreamLine) error {
				if len(line.Boxes) == 0 {
					// FR-28 -- name the images, not just the count: "12 with
					// nothing found" is a number, a list is something a person
					// can act on.
					empty = append(empty, line.Image)
				} else {
					boxes := make([]store.Box, len(line.Boxes))
					for i, d := range line.Boxes {
						boxes[i] = store.Box{Cls: d.Cls, Box: d.Box}
					}
					if _, err := s.Store.WriteBoxes(ctx, inputDir, store.KindPool,
						line.Image, boxes, nil, false); err != nil {
						return err
					}
					written++
					autoPaths = append(autoPaths, line.Image)
				}
				done++
				s.Jobs.Tick(jobID, done)
				return nil
			})
		if err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		if err := s.Store.MarkAuto(ctx, inputDir, autoPaths); err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		bank, err := s.bankSummaryCtx(ctx, inputDir, stateDir)
		if err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		s.Jobs.Finish(jobID, map[string]any{
			"written": written, "no_detection": len(empty),
			"no_detection_images": empty, "bank": bank,
		})
	}()

	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "total": len(paths)})
	return nil
}

// Reembed switches an already-taught project to a different checkpoint by
// re-extracting every stored instance under it -- the only sanctioned way to
// change a bank's model after its first label. Labels in PostgreSQL are never
// touched; only the prompt bank's vectors and its model change.
//
// Job result: {"bank": dict}
func (s *Server) Reembed(w http.ResponseWriter, r *http.Request) error {
	var req jobRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	info, err := s.VPE.TotalInstances(r.Context(), stateDir)
	if err != nil {
		return err
	}
	if info.Model == nil {
		return errStatus(http.StatusBadRequest,
			"this project has no model yet -- just label normally, no need to reembed")
	}
	if req.ModelID == *info.Model {
		return errStatus(http.StatusBadRequest, fmt.Sprintf("already using %s", pyRepr(req.ModelID)))
	}
	// Fail fast on a bad id, before spawning a job that would only discover it
	// after loading nothing.
	if !s.Catalog.Has(req.ModelID) {
		return errStatus(http.StatusBadRequest, fmt.Sprintf("unknown model %s", pyRepr(req.ModelID)))
	}

	// Counted in instances, not images: one image can have taught several.
	jobID := s.Jobs.Create(info.Total)
	modelID := req.ModelID

	go func() {
		ctx, cancel := jobContext()
		defer cancel()
		err := s.VPE.ReembedStream(ctx, stateDir, modelID, func(line vpe.StreamLine) error {
			s.Jobs.Tick(jobID, line.DoneCount)
			return nil
		})
		if err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		bank, err := s.bankSummaryCtx(ctx, inputDir, stateDir)
		if err != nil {
			s.Jobs.Fail(jobID, err.Error())
			return
		}
		s.Jobs.Finish(jobID, map[string]any{"bank": bank})
	}()

	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "total": info.Total})
	return nil
}

// withThresholds merges the conf settings into the result, matching the
// Python's `metrics.evaluate(...) | {"conf": ..., "conf_by_class": ...}`.
func withThresholds(r metrics.Result, conf float64, byClass map[string]float64) map[string]any {
	if byClass == nil {
		byClass = map[string]float64{}
	}
	return map[string]any{
		"overall": r.Overall, "per_class": r.PerClass, "per_image": r.PerImage,
		"images": r.Images, "iou": r.IoU,
		"conf": conf, "conf_by_class": byClass,
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
