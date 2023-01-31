package main

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// TODO: create API with create, delete methods
// TODO: add GRPC, websocket examples
type album struct {
	Title string `json:"title"`
}

var albumLock sync.Mutex
var albums = []album{
	{Title: "Blue Train"},
	{Title: "Jeru"},
	{Title: "Sarah Vaughan and Clifford Brown"},
}

func getBooks(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, albums)
}

func createBook(c *gin.Context) {
	albumLock.Lock()
	defer albumLock.Unlock()

	var newAlbum album

	// Call BindJSON to bind the received JSON to
	// newAlbum.
	if err := c.BindJSON(&newAlbum); err != nil {
		return
	}

	c.Request.Close = true

	// Add the new album to the slice.
	albums = append(albums, newAlbum)
	c.IndentedJSON(http.StatusCreated, newAlbum)
}

func main() {
	router := gin.Default()

	router.GET("/books", getBooks)
	router.POST("/books", createBook)

	router.Run("localhost:8080")
}
