package face

import (
	"errors"
	"io"
)

type APIConfig struct {
	// Some config.
}

type API struct {
	config APIConfig
}

func NewAPI(c APIConfig) *API {
	return &API{config: c}
}

const addUserPath = ""

type addUserResponseData struct {
	// Some data.
}

func (a *API) AddUser(photoFilePath string) (string, error) {
	// Some algorithm.
	return "", nil
}

const addUserPhotoPath = "/some/path"

type addUserPhotoResponseData struct {
	// Some data.
}

func (a *API) AddUserPhoto(userID string, photoFilePath string) error {
	// Some algorithm.
	return nil
}

const recognizeUserPath = "/some/path"

type recognizeUserResponseData struct {
	// Some data.
}

var (
	ErrFaceNotFound = errors.New("face not found")
)

func (a *API) RecognizeUser(photo io.Reader) (string, error) {
	// Some algorithm.
	return "", nil
}

const removeUserRequestPath = "/some/path"

type removeUserRequestData struct {
	// Some data.
}

type removeUserResponseData struct {
	// Some data.
}

func (a *API) RemoveUser(userID string) error {
	// Some algorithm.
	return nil
}
