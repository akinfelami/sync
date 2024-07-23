package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/skratchdot/open-golang/open"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"

	_ "github.com/joho/godotenv/autoload"
)

type Playlist struct {
	Tracks []Track
}

type Track struct {
	Artists []string
	Title   string
}

const redirectURI = "http://localhost:8080/callback"

var (
	auth = spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistReadPrivate, spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopeUserReadPrivate),
	)
	state  = "abc123"
	client *spotify.Client
	mux    sync.RWMutex
)

func setClient(c *spotify.Client) {
	mux.Lock()
	defer mux.Unlock()
	client = c
}

func getClient() *spotify.Client {
	mux.RLock()
	defer mux.RUnlock()
	return client
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if getClient() == nil {
			url := auth.AuthURL(state)
			c.Redirect(http.StatusTemporaryRedirect, url)
			c.Abort()
			return
		}
		c.Next()
	}
}

func authRedirectHandler(c *gin.Context) {
	token, err := auth.Token(c, state, c.Request)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Couldn't get token"})
	}
	// create a client using the specified token
	client := spotify.New(auth.Client(c, token))
	setClient(client)

	user, err := client.CurrentUser(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Internal Server Error",
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome " + user.DisplayName,
	})
}

func getPlaylists(c *gin.Context) spotify.ID {

	client := getClient()
	response, err := client.CurrentUsersPlaylists(context.Background(), spotify.Limit(50))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Internal Server Error",
		})
		log.Println(err)
	}
	for _, playlist := range response.Playlists {
		name := playlist.Name
		if match := regexp.MustCompile(`^daylist\s*\W\s*.*$`); match.MatchString(name) {
			return playlist.ID
		}
	}
	return ""
}

func getDaylist(c *gin.Context) {

	client := getClient()

	playlistID := getPlaylists(c)
	if playlistID == "" {
		c.JSON(http.StatusOK, gin.H{
			"message": "Sorry, We couldn't find a daylist playlist",
		})
		return
	}

	response, err := client.GetPlaylistItems(context.Background(), playlistID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Internal Server Error",
		})
		log.Println(err)
		return
	}

	var playlist Playlist
	for _, item := range response.Items {
		var artists []string
		for _, artist := range item.Track.Track.Artists {
			artists = append(artists, artist.Name)
		}
		playlist.Tracks = append(playlist.Tracks, Track{
			Title:   item.Track.Track.Name,
			Artists: artists,
		})
	}

	c.JSON(200, gin.H{
		"message": "success",
		"data":    playlist,
	})
}

func main() {

	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello World!",
		})
	})

	r.GET("/callback", authRedirectHandler)
	r.GET("/daylist", authMiddleware(), getDaylist)

	url := auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:\n\n", url)

	open.Start(url)

	err := r.Run(":8080")
	if err != nil {
		log.Fatal(err)
	}
}
