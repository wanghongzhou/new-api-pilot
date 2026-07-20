package common

type Identity struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`
	Status             int    `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
	SessionVersion     int    `json:"session_version"`
}
