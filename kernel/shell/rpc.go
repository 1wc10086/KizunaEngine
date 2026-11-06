package shell

import (
	"encoding/json"
	"fmt"

	"kizuna/kernel/core"
	"kizuna/kernel/translator"
)

func Invoke(input string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = errPayload(fmt.Sprintf("panic: %v", r))
		}
	}()
	var req request
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return errPayload("invalid request: " + err.Error())
	}
	switch req.Op {
	case "ping":
		return okPayload(map[string]any{"version": Version, "features": listFeatures()})
	case "features":
		return okPayload(map[string]any{"features": listFeatures()})
	case "detect":
		if req.Path == "" {
			return errPayload("missing path")
		}
		dr, err := core.Detect(req.Path)
		if err != nil {
			return errPayload(err.Error())
		}
		return okPayload(map[string]any{"detect": dr})
	case "process":
		return handleProcess(req)
	case "translate.detect":
		return handleTranslateDetect(req)
	case "translate.start":
		return handleTranslateStart(req)
	case "translate.poll":
		return handleTranslatePoll(req)
	case "translate.cancel":
		return okPayload(map[string]any{"canceled": cancelTask(req.TaskID)})
	default:
		if req.Op == "" {
			return errPayload("missing op")
		}
		return errPayload("unknown op: " + req.Op)
	}
}

func handleProcess(req request) string {
	if req.Feature == "" {
		return errPayload("missing feature")
	}
	if req.Action == "" {
		return errPayload("missing action")
	}
	if req.Input == "" {
		return errPayload("missing input")
	}
	if req.Output == "" {
		return errPayload("missing output")
	}
	f, ok := core.Find(req.Feature)
	if !ok {
		return errPayload("unknown feature: " + req.Feature)
	}
	opt := core.Options{TextEncoding: req.Encoding, Extra: req.Extra}
	res, err := f.Run(req.Action, req.Input, req.Output, opt)
	if err != nil {
		return errPayload(err.Error())
	}
	return okPayload(map[string]any{"result": res})
}

func handleTranslateDetect(req request) string {
	if req.Path == "" {
		return errPayload("missing path")
	}
	dr, err := core.Detect(req.Path)
	if err != nil {
		return errPayload(err.Error())
	}
	formats := make([]formatDTO, 0, len(dr.Candidates))
	seen := make(map[string]bool, len(dr.Candidates))
	for _, c := range dr.Candidates {
		key := c.FeatureID + "\x00" + c.Format
		if seen[key] {
			continue
		}
		seen[key] = true
		formats = append(formats, formatDTO{
			FeatureID: c.FeatureID,
			Format:    c.Format,
			Name:      c.FeatureName,
		})
	}
	return okPayload(map[string]any{"formats": formats})
}

func handleTranslateStart(req request) string {
	treq := translator.Request{
		Source:         req.Source,
		Target:         req.Target,
		Feature:        req.Feature,
		SourceEncoding: req.SourceEncoding,
		TargetEncoding: req.TargetEncoding,
		Provider:       req.Provider,
		Params:         req.Params,
	}
	if treq.Source == "" {
		return errPayload("missing source")
	}
	if treq.Feature == "" {
		return errPayload("missing feature")
	}
	if treq.Target == "" {
		treq.Target = treq.Source + ".out"
	}
	id := startTask(treq)
	return okPayload(map[string]any{"taskId": id})
}

func handleTranslatePoll(req request) string {
	done, events, ok := pollTask(req.TaskID)
	if !ok {
		return errPayload(fmt.Sprintf("unknown taskId: %d", req.TaskID))
	}
	return okPayload(map[string]any{"done": done, "events": events})
}
