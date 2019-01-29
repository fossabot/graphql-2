// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package graphql

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

// A book is a book on Goodreads.
type Book struct {
	ID    string `json:"id"`
	URI   string `json:"uri"`
	Title string `json:"title"`
}

func (Book) IsLinkable() {}

// Comment is an undefined type reserved for the future.
type Comment struct {
	ID string `json:"id"`
}

type EditedPost struct {
	Content  string    `json:"content"`
	Title    string    `json:"title"`
	Datetime time.Time `json:"datetime"`
	Draft    bool      `json:"draft"`
}

type Linkable interface {
	IsLinkable()
}

type NewLink struct {
	Title       string     `json:"title"`
	URI         string     `json:"uri"`
	Description string     `json:"description"`
	Tags        []string   `json:"tags"`
	Created     *time.Time `json:"created"`
}

type NewPost struct {
	Content  *string    `json:"content"`
	Title    *string    `json:"title"`
	Datetime *time.Time `json:"datetime"`
	Draft    *bool      `json:"draft"`
}

type NewStat struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type NewTweet struct {
	FavoriteCount int       `json:"favorite_count"`
	Hashtags      []string  `json:"hashtags"`
	ID            string    `json:"id"`
	Posted        time.Time `json:"posted"`
	RetweetCount  int       `json:"retweet_count"`
	Symbols       []string  `json:"symbols"`
	Text          string    `json:"text"`
	Urls          []string  `json:"urls"`
	ScreenName    string    `json:"screen_name"`
	UserMentions  []string  `json:"user_mentions"`
}

type Searchable interface {
	IsSearchable()
}

// A stat is a key value pair of two interesting strings.
type Stat struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleNormal Role = "normal"
)

func (e Role) IsValid() bool {
	switch e {
	case RoleAdmin, RoleNormal:
		return true
	}
	return false
}

func (e Role) String() string {
	return string(e)
}

func (e *Role) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = Role(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid Role", str)
	}
	return nil
}

func (e Role) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
