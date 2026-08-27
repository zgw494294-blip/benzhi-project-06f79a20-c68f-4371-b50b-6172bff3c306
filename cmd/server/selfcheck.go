package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
)

type checkClient struct {
	base   string
	client *http.Client
}

func (c checkClient) request(ctx context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, data)
	}
	if target != nil && len(data) > 0 {
		return json.Unmarshal(data, target)
	}
	return nil
}

func runSelfcheck(ctx context.Context, baseURL string) error {
	c := checkClient{base: baseURL, client: &http.Client{}}
	var view application.DossierView
	if err := c.request(ctx, "GET", "/healthz", nil, nil); err != nil {
		return err
	}
	create := application.CreateDossierCommand{Title: "自检口述历史档案", ParticipantName: "参与者甲", InterviewerName: "访谈员乙", SessionDate: "2026-08-27", MaterialSummary: "用于验证从授权建档到分级开放的完整业务流程。", ConsentScope: "public", Actor: "访谈员乙"}
	if err := c.request(ctx, "POST", "/api/dossiers", create, &view); err != nil {
		return err
	}
	id := view.Snapshot.Dossier.DossierID
	edit := application.ReviseDossierCommand{Version: view.Snapshot.Dossier.Version, Title: "自检口述历史档案", ParticipantName: "参与者甲", InterviewerName: "访谈员乙", SessionDate: "2026-08-26", MaterialSummary: "用于验证校订、证据链和分级开放的完整业务流程。", ConsentScope: "public", Actor: "访谈员乙", Reason: "纠正访谈日期与材料摘要"}
	if err := c.request(ctx, "PATCH", "/api/dossiers/"+id, edit, &view); err != nil {
		return err
	}
	revision := application.SubmitRevisionCommand{Version: view.Snapshot.Dossier.Version, SourceDigest: "audio-source-selfcheck", SubmittedBy: "访谈员乙", Segments: []application.SegmentInput{{SegmentID: "S001", Sequence: 1, Text: "可公开的社区生活回忆。"}, {SegmentID: "S002", Sequence: 2, Text: "涉及第三方姓名的原始叙述。"}}}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/revisions", revision, &view); err != nil {
		return err
	}
	var history any
	if err := c.request(ctx, "GET", "/api/dossiers/"+id+"/revisions/history?from=1&to=1", nil, &history); err != nil {
		return err
	}
	annotations := application.AnnotateCommand{Version: view.Snapshot.Dossier.Version, Actor: "访谈员乙", Annotations: []application.AnnotationInput{{SegmentID: "S001", ProposedAccessLevel: "public"}, {SegmentID: "S002", SensitivityTags: []string{"name", "third_party"}, ProposedAccessLevel: "researcher"}}}
	var annotationPreflight any
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/annotations/preflight", annotations, &annotationPreflight); err != nil {
		return err
	}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/annotations", annotations, &view); err != nil {
		return err
	}
	draft := application.SaveReviewDraftCommand{Version: view.Snapshot.Dossier.Version, RevisionID: view.CurrentRevision.RevisionID, Participant: "参与者甲", Decisions: []application.DecisionInput{{SegmentID: "S001", DecisionType: "confirm"}}}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/reviews/draft", draft, &view); err != nil {
		return err
	}
	review := application.ReviewCommand{Version: view.Snapshot.Dossier.Version, RevisionID: view.CurrentRevision.RevisionID, Participant: "参与者甲", Decisions: []application.DecisionInput{{SegmentID: "S002", DecisionType: "text_change", RequestedText: "涉及一位旧同事的匿名叙述。", Reason: "请隐去第三方姓名"}}}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/reviews", review, &view); err != nil {
		return err
	}
	var requestID string
	for _, decision := range view.Snapshot.Decisions {
		if decision.DecisionType == "text_change" && !decision.IsClosed() {
			requestID = decision.DecisionID
		}
	}
	if requestID == "" {
		return fmt.Errorf("未生成文字修订请求")
	}
	resolve := application.ResolveCommand{Version: view.Snapshot.Dossier.Version, Actor: "编研员丙", Resolution: "接受匿名化建议并替换段落", ReplacementText: "涉及一位旧同事的匿名叙述。"}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/decisions/"+requestID+"/resolve", resolve, &view); err != nil {
		return err
	}
	reconfirm := application.ReviewCommand{Version: view.Snapshot.Dossier.Version, RevisionID: view.CurrentRevision.RevisionID, Participant: "参与者甲", Decisions: []application.DecisionInput{{SegmentID: "S001", DecisionType: "confirm"}, {SegmentID: "S002", DecisionType: "confirm"}}}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/reviews", reconfirm, &view); err != nil {
		return err
	}
	var sealPreflight application.SealPreflight
	if err := c.request(ctx, "GET", "/api/dossiers/"+id+"/seal/preflight", nil, &sealPreflight); err != nil {
		return err
	}
	if !sealPreflight.CanSeal {
		return fmt.Errorf("封存预检存在阻断项: %#v", sealPreflight.Blockers)
	}
	seal := application.SealCommand{Version: view.Snapshot.Dossier.Version, ConfirmationDigest: sealPreflight.ConfirmationDigest, Actor: "编研员丙", Reason: "请求均已关闭且授权边界一致"}
	if err := c.request(ctx, "POST", "/api/dossiers/"+id+"/seal", seal, &view); err != nil {
		return err
	}
	if view.Snapshot.Dossier.Status != "released" || view.Snapshot.Manifest == nil {
		return fmt.Errorf("档案未进入已开放状态")
	}
	var publicCopy application.ReadingCopy
	if err := c.request(ctx, "GET", "/api/dossiers/"+id+"/reading?level=public", nil, &publicCopy); err != nil {
		return err
	}
	if len(publicCopy.Segments) != 1 || strings.Contains(marshalText(publicCopy), "旧同事") {
		return fmt.Errorf("公开副本包含不应公开的段落")
	}
	var researcherCopy application.ReadingCopy
	if err := c.request(ctx, "GET", "/api/dossiers/"+id+"/reading?level=researcher", nil, &researcherCopy); err != nil {
		return err
	}
	if len(researcherCopy.Segments) != 2 || !researcherCopy.AuditValid {
		return fmt.Errorf("研究者副本或审计链验证失败")
	}
	return nil
}

func marshalText(v any) string { data, _ := json.Marshal(v); return string(data) }
