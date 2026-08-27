package domain

import (
	"fmt"
	"strings"
	"time"
)

type AccessLevel string

const (
	AccessPublic     AccessLevel = "public"
	AccessResearcher AccessLevel = "researcher"
	AccessRestricted AccessLevel = "restricted"
	AccessClosed     AccessLevel = "closed"
)

var accessRank = map[AccessLevel]int{
	AccessPublic: 0, AccessResearcher: 1, AccessRestricted: 2, AccessClosed: 3,
}

func ParseAccessLevel(raw string) (AccessLevel, error) {
	v := AccessLevel(strings.TrimSpace(raw))
	if _, ok := accessRank[v]; !ok {
		return "", Invalid("invalid_access_level", "开放级别必须为 public、researcher、restricted 或 closed", "accessLevel")
	}
	return v, nil
}

// Allows 表示当前访问者的权限是否足以读取目标段落；数字越大权限越高。
func (viewer AccessLevel) Allows(required AccessLevel, embargoUntil *time.Time, now time.Time) bool {
	if required == AccessClosed {
		return false
	}
	if embargoUntil != nil && now.Before(*embargoUntil) && viewer != AccessRestricted {
		return false
	}
	return accessRank[viewer] >= accessRank[required]
}

// WithinConsent 检查建议级别是否不比整体授权更开放。
func (level AccessLevel) WithinConsent(consent AccessLevel) bool {
	return accessRank[level] >= accessRank[consent]
}

func (a AccessLevel) Validate() error {
	if _, ok := accessRank[a]; !ok {
		return fmt.Errorf("%w: 未知开放级别 %q", ErrValidation, a)
	}
	return nil
}
