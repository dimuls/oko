package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"

	"github.com/dimuls/oko/entity"
	"github.com/dimuls/oko/face"
)

func main() {

	app := &cli.App{
		Name:  "users",
		Usage: "управление пользователями",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "host-config-path",
				Aliases:  []string{"c"},
				Usage:    "путь до конфига хоста",
				Required: true,
				Value:    "configs",
			},
		},
		Commands: cli.Commands{
			{
				Name:   "add_user_photos",
				Usage:  "добавить фотографии пользователя, создаёт пользователя при необходимости",
				Action: addUserPhotos,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "user-name",
						Aliases:  []string{"u"},
						Usage:    "имя пользователя",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "photo-path",
						Aliases:  []string{"p"},
						Usage:    "путь до фотографии или папки с фотографиями, флаг можно указать несколько раз",
						Required: true,
					},
					// Some face API params.
				},
			},
			{
				Name:   "list_users",
				Usage:  "вывести список пользователей",
				Action: listUsers,
			},
			{
				Name:   "remove_user",
				Usage:  "удалить пользователя",
				Action: removeUser,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "user-name",
						Aliases:  []string{"u"},
						Usage:    "имя пользователя, который будет удалён",
						Required: true,
					},
					// Some face API params.
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func loadHost(hostConfigPath string) (entity.Host, error) {
	f, err := os.Open(hostConfigPath)
	if err != nil {
		return entity.Host{}, err
	}

	defer f.Close()

	var h entity.Host

	err = yaml.NewDecoder(f).Decode(&h)
	if err != nil {
		return entity.Host{}, err
	}

	return h, nil
}

func saveHost(h entity.Host, hostConfigPath string) error {
	f, err := os.Create(hostConfigPath)
	if err != nil {
		return err
	}

	defer f.Close()

	return yaml.NewEncoder(f).Encode(h)
}

var allowedExtensions = []string{"jpg", "jpeg", "png"}

func hasAllowedExtension(filePath string) bool {
	ext := strings.TrimLeft(strings.ToLower(filepath.Ext(filePath)), ".s")
	for _, e := range allowedExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

func addUserPhotos(c *cli.Context) error {
	hostConfigPath := c.String("host-config-path")
	userName := c.String("user-name")
	photosPaths := c.StringSlice("photo-path")
	// Some face API params.

	h, err := loadHost(hostConfigPath)
	if err != nil {
		return cli.NewExitError("не удалось открыть конфиг хоста: "+err.Error(), 1)
	}

	faceAPI := face.NewAPI(face.APIConfig{
		// Some face API params.
	})

	var photoFilePaths []string

	for _, pp := range photosPaths {
		stat, err := os.Stat(pp)
		if err != nil {
			fmt.Printf("не удалось прочитать путь %s: %v\n", pp, err)
			continue
		}

		if stat.IsDir() {
			files, err := ioutil.ReadDir(pp)
			if err != nil {
				fmt.Printf("не удалось получить список файлов папки %s: %v\n", pp, err)
				continue
			}
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				if !hasAllowedExtension(f.Name()) {
					continue
				}
				photoFilePaths = append(photoFilePaths, path.Join(pp, f.Name()))
			}
		} else {
			photoFilePaths = append(photoFilePaths, pp)
		}
	}

	if h.Users == nil {
		h.Users = map[string]string{}
	}

	userID, exists := h.Users[userName]

	for _, f := range photoFilePaths {

		if !exists {
			userID, err = faceAPI.AddUser(f)
			if err != nil {
				fmt.Printf("не удалось добавить пользователя с фотографией %s в face API: %v\n", f, err)
				continue
			}

			h.Users[userName] = userID

			err = saveHost(h, hostConfigPath)
			if err != nil {
				return cli.NewExitError(fmt.Sprintf(
					"не удалось сохранить конфиг хоста %s вместе с новым пользователем %s: %v\n",
					hostConfigPath, userID, err), 2)
			}

			exists = true
			continue
		}

		err = faceAPI.AddUserPhoto(userID, f)
		if err != nil {
			fmt.Printf("не удалось добавить фотографию %s пользователю %s: %v\n", f, userID, err)
		}
	}

	return nil
}

func listUsers(c *cli.Context) error {
	hostConfigPath := c.String("host-config-path")

	h, err := loadHost(hostConfigPath)
	if err != nil {
		return cli.NewExitError("не удалось открыть конфиг хоста: "+err.Error(), 1)
	}

	if h.Users == nil {
		return nil
	}

	for userName, userID := range h.Users {
		fmt.Printf("%s: %s\n", userName, userID)
	}

	return nil
}

func removeUser(c *cli.Context) error {
	hostConfigPath := c.String("host-config-path")
	userName := c.String("user-name")
	// Some face API params.

	h, err := loadHost(hostConfigPath)
	if err != nil {
		return cli.NewExitError("не удалось открыть конфиг хоста: "+err.Error(), 1)
	}

	if h.Users == nil {
		return nil
	}

	userID, exists := h.Users[userName]
	if !exists {
		return nil
	}

	faceAPI := face.NewAPI(face.APIConfig{
		// Some face API params.
	})

	err = faceAPI.RemoveUser(userID)
	if err != nil {
		return cli.NewExitError(fmt.Sprintf("не удалось удалить пользователя в face API: %v", err), 2)
	}

	delete(h.Users, userName)

	err = saveHost(h, hostConfigPath)
	if err != nil {
		return cli.NewExitError(fmt.Sprintf(
			"не удалось сохранить конфиг хоста %s вместе с удалённым пользователем %s: %v\n",
			hostConfigPath, userName, err), 2)
	}

	return nil
}
