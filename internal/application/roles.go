package application

import (
	"fmt"
	"strings"
)

type Role string

const (
	RoleOperator Role = "operator"
	RoleReviewer Role = "reviewer"
)

type Principal struct {
	Name string `json:"name"`
	Role Role   `json:"role"`
}

func (p Principal) Validate(required Role) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("缺少操作人员姓名")
	}
	if p.Role != required {
		return fmt.Errorf("操作要求角色 %s，当前角色为 %s", required, p.Role)
	}
	return nil
}
