package mock

type Status struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}
type Response struct {
	Status Status `json:"Status"`
}

type Result struct {
	Data interface{} `json:"Data"`
}

type MockResponse struct {
	Response
	Result Result `json:"Result"`
}
