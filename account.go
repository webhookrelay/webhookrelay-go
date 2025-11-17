package webhookrelay

import (
	"encoding/json"
	"net/http"
)

type UserInfo struct {
	Status string `json:"status"`
	Data   struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		AuthType string `json:"auth_type"`
		PlanID   string `json:"plan_id"`
		Role     string `json:"role"`
	} `json:"data"`
}

// UserInfo returns current user details
func (api *API) UserInfo() (*UserInfo, error) {

	resp, err := api.makeRequest(http.MethodGet, "/user/info", nil)
	if err != nil {
		return nil, err
	}

	var userInfo UserInfo
	if err := json.Unmarshal(resp, &userInfo); err != nil {
		return nil, err
	}
	return &userInfo, nil
}
