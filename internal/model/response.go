package model

type SuccessResponse struct {
	Data interface{} `json:"data"`
}

type PaginatedResponse struct {
	Data     interface{} `json:"data"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
