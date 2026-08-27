package audit

import (
	"fmt"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type Verification struct {
	Valid      bool   `json:"valid"`
	Count      int    `json:"count"`
	HeadDigest string `json:"headDigest"`
}

func Verify(events []domain.AuditEvent) (Verification, error) {
	result := Verification{Valid: true, Count: len(events)}
	var previous string
	for i, e := range events {
		expected := int64(i + 1)
		if e.Sequence != expected {
			return Verification{}, fmt.Errorf("审计序号不连续：期望 %d，得到 %d", expected, e.Sequence)
		}
		if e.PreviousDigest != previous {
			return Verification{}, fmt.Errorf("审计事件 %d 的前序摘要不匹配", expected)
		}
		if eventDigest(e) != e.EventDigest {
			return Verification{}, fmt.Errorf("审计事件 %d 摘要不匹配", expected)
		}
		previous = e.EventDigest
	}
	result.HeadDigest = previous
	return result, nil
}
