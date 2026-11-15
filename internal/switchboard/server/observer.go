package server

type ObserverRequest struct {
	Kind string `json:"kind"`
}

type ObserverResponse struct {
	Token string `json:"token"`
}
