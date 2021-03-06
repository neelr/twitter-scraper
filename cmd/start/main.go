package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/badoux/goscraper"
	"github.com/brianloveswords/airtable"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/joho/godotenv"
	"mvdan.cc/xurls/v2"
)

type TweetRecord struct {
	airtable.Record
	Fields struct {
		Title       string
		URL         string
		DisplayName string `json:"Display Name"`
		Description string
	}
}

type FollowingRecord struct {
	airtable.Record
	Fields struct {
		ID string
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Keepin me alive!")
}

// Finds item in array
func Find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

func main() {

	// Load .env file
	err := godotenv.Load()

	if err != nil {
		fmt.Println(err)
		return
	}

	// Create airtable client
	airtableClient := airtable.Client{
		APIKey: os.Getenv("AIRTABLE"),
		BaseID: os.Getenv("BASE"),
	}

	scrapedTweets := airtableClient.Table("Scraped Tweets")
	followingTable := airtableClient.Table("Following")

	// Get all ID's to follow
	allID := []string{}
	following := &[]FollowingRecord{}

	err = followingTable.List(following, &airtable.Options{})

	if err != nil {
		fmt.Println(err)
		return
	}

	for _, ID := range *following {
		allID = append(allID, ID.Fields.ID)
	}

	// Create String url parser for tweet
	urlParse := xurls.Strict()

	// create oauth http client
	config := oauth1.NewConfig(os.Getenv("API_KEY"), os.Getenv("API_SECRET_KEY"))
	token := oauth1.NewToken(os.Getenv("ACCESS_TOKEN"), os.Getenv("ACCESS_TOKEN_SECRET"))
	httpClient := config.Client(oauth1.NoContext, token)

	// Create twitter client from http
	client := twitter.NewClient(httpClient)

	// Params for stream
	params := &twitter.StreamFilterParams{
		Follow:        allID,
		StallWarnings: twitter.Bool(false),
	}

	// Create stream
	stream, err := client.Streams.Filter(params)

	if err != nil {
		fmt.Println(err)
		return
	}

	// Create a demux handler
	demux := twitter.NewSwitchDemux()

	demux.Tweet = func(tweet *twitter.Tweet) {
		fmt.Println("New Tweet from " + tweet.User.Name)

		_, fromUser := Find(allID, tweet.User.IDStr)

		if !fromUser {
			return
		}

		text := tweet.Text

		if tweet.ExtendedTweet != nil {
			text = tweet.ExtendedTweet.FullText
		}

		// Get all url's in tweet text
		urls := urlParse.FindAllString(text, -1)

		for _, url := range urls {
			// Scrape the url
			s, err := goscraper.Scrape(url, 1)

			if err != nil {
				fmt.Println(err)
				return
			}

			// Make the record
			tweetData := &TweetRecord{}
			airtable.NewRecord(tweetData, airtable.Fields{
				"Title":       s.Preview.Title,
				"Description": s.Preview.Description,
				"URL":         s.Preview.Link,
				"DisplayName": tweet.User.Name,
			})

			// Create the record
			err = scrapedTweets.Create(tweetData)

			if err != nil {
				fmt.Println(err)
				return
			}
		}
	}

	demux.DM = func(dm *twitter.DirectMessage) {
		// Don't need DM's
		return
	}

	fmt.Println("Listening....")

	// Read stream (loops forever until disconnect)
	go func(stream *twitter.Stream, demux twitter.SwitchDemux) {
		for message := range stream.Messages {
			demux.Handle(message)
		}
	}(stream, demux)

	// create a server for repl.it hosting keep alive
	http.HandleFunc("/", rootHandler)
	http.ListenAndServe(":8080", nil)
}
