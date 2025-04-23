package mock

type Status struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}
type Response struct {
	Status Status `json:"Status"`
}
