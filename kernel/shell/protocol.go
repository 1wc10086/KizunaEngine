package shell

import (
	"encoding/json"

	"kizuna/kernel/core"
	"kizuna/kernel/translator"
)

const Version = "kizuna-kernel/1.0.0"

type request struct {
	Op             string                  `json:"op"`
	Path           string                  `json:"path"`
	Feature        string                  `json:"feature"`
	Action         string                  `json:"action"`
	Input          string                  `json:"input"`
	Output         string                  `json:"output"`
	Encoding       string                  `json:"encoding"`
	Extra          map[string]string       `json:"extra"`
	Source         string                  `json:"source"`
	Target         string                  `json:"target"`
	SourceEncoding string                  `json:"sourceEncoding"`
	TargetEncoding string                  `json:"targetEncoding"`
	Provider       translator.ProviderSpec `json:"provider"`
	Params         translator.Params       `json:"params"`
	TaskID         int64                   `json:"taskId"`
}

type handlerDTO struct {
	Action       string   `json:"action"`
	Label        string   `json:"label"`
	Suffixes     []string `json:"suffixes"`
	OutputSuffix string   `json:"outputSuffix"`
}

type featureDTO struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Target   string       `json:"target"`
	Handlers []handlerDTO `json:"handlers"`
}

type formatDTO struct {
	FeatureID string `json:"featureId"`
	Format    string `json:"format"`
	Name      string `json:"name"`
}

func listFeatures() []featureDTO {
	fs := core.Features()
	out := make([]featureDTO, 0, len(fs))
	for _, f := range fs {
		target := "file"
		if f.Target() == core.TargetDirectory {
			target = "dir"
		}
		hs := f.Handlers()
		dtos := make([]handlerDTO, 0, len(hs))
		for _, h := range hs {
			sfx := h.Suffixes
			if sfx == nil {
				sfx = []string{}
			}
			dtos = append(dtos, handlerDTO{
				Action:       h.Action,
				Label:        h.Label,
				Suffixes:     sfx,
				OutputSuffix: h.OutputSuffix,
			})
		}
		out = append(out, featureDTO{
			ID:       f.ID(),
			Name:     f.Name(),
			Target:   target,
			Handlers: dtos,
		})
	}
	return out
}

func okPayload(fields map[string]any) string {
	fields["ok"] = true
	data, err := json.Marshal(fields)
	if err != nil {
		return `{"ok":false,"error":"encode response"}`
	}
	return string(data)
}

func errPayload(msg string) string {
	data, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
	return string(data)
}
