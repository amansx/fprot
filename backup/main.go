package main

import "os"
import "fmt"
import "sort"
import "regexp"
import "time"
import "strings"
import "sync"
import "io/ioutil"
import "encoding/json"
import "github.com/dghubble/go-twitter/twitter"
import "golang.org/x/oauth2"
import "golang.org/x/oauth2/clientcredentials"
import "github.com/joho/godotenv"
import "github.com/cavaliercoder/grab"
import "github.com/aymerick/raymond"


var tmpl string
var hashtag = regexp.MustCompile(` #(\w+)`)
var handlereg = regexp.MustCompile(` @(\w+)`)
var hashtagb = regexp.MustCompile(`^#(\w+)`)
var handleregb = regexp.MustCompile(`^@(\w+)`)
var urlreg = regexp.MustCompile(`(https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*))`)

func init() {
	loc, _ := time.LoadLocation("Asia/Kolkata")

	raymond.RegisterHelper("date", func(dt time.Time) raymond.SafeString {
	    return raymond.SafeString(dt.In(loc).Format("Jan 2, 2006 3:04pm"))
	})

	raymond.RegisterHelper("even", func(i int, options *raymond.Options) raymond.SafeString {
		if i%2 == 0 {
			return raymond.SafeString(options.Fn())
		} else {
			return ""
		}
	})

	raymond.RegisterHelper("twitter", func(content string, options *raymond.Options) raymond.SafeString {
		content = strings.Replace(content, "\n", "<br>", -1)
		content = urlreg.ReplaceAllString(content, `<br><a target="_blank" href="$1">$1</a>`)
		content = hashtag.ReplaceAllString(content, ` <a target="_blank" href="https://twitter.com/search?q=%23$1">#$1</a>`)
		content = handlereg.ReplaceAllString(content, ` <a target="_blank" href="https://twitter.com/$1">@$1</a> `)
		content = hashtagb.ReplaceAllString(content, `<a target="_blank" href="https://twitter.com/search?q=%23$1">#$1</a>`)
		content = handleregb.ReplaceAllString(content, `<a target="_blank" href="https://twitter.com/$1">@$1</a> `)

		return raymond.SafeString(content)
	})

	btmpl, _ := ioutil.ReadFile("../template.html")
	tmpl = string(btmpl)

}

var wg sync.WaitGroup
var client *twitter.Client
func newFalse() *bool {
    b := false
    return &b
}

func newTrue() *bool {
    b := true
    return &b
}

type Media struct {
	ID int64
	Type string
	URL string
	Quality int
}
func NewMedia(i int64, t string, u string, q int) Media {
	return Media{ID: i, Type: t, URL: u, Quality: q}
}

func Download(handle string, m Media) {
	var e string
	if m.Type == "video" {
		e = "mp4"
		return
	} else {
		e = "jpeg"
	}
	// fmt.Println("+", m.URL)
	fn := fmt.Sprintf("../tweets/%v/%v.%v", handle, m.ID, e)
	grab.Get(fn, m.URL)
	fmt.Println("√", fn, m.URL)
}

type Tweet struct {
	ID int64
	DateTime time.Time
	Text string
	Media map[int64]Media
}
func (this *Tweet) AddMedia(handle string, m Media) {
	if this.Media[m.ID].Quality <= m.Quality {
		this.Media[m.ID] = m
		go Download(handle, m)
	}
}
func NewTweet(id int64, dt time.Time, text string) *Tweet {
	return &Tweet{
		ID: id, 
		DateTime: dt, 
		Text: text, 
		Media: map[int64]Media{},
	}
}



type TweetSuperMap map[int64]TweetMap
func (this TweetSuperMap) SortedKeys() ([]int64) {
	keys := make([]int64, len(this))
	i := 0
	for k := range this {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

type TweetMap map[int64]*Tweet
func (this TweetMap) SortedKeys() ([]int64) {
	keys := make([]int64, len(this))
	i := 0
	for k := range this {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}
// func Render(handle string, tweets TweetMap) {
// 	fn := fmt.Sprintf("../tweets/%v/index.html", handle)
// 	result, _ := raymond.Render(tmpl, map[string]TweetMap{
// 		"tweets": tweets,
// 	})

// 	err := ioutil.WriteFile(fn, []byte(result), 0777)
// 	if err != nil {
// 		fmt.Println(err)
// 	}
// }
func Render(handle string, tweets [][]*Tweet) {
	fn := fmt.Sprintf("../tweets/%v/index.html", handle)
	result, _ := raymond.Render(tmpl, map[string]interface{}{
		"tweets": tweets,
		"handle": handle,
	})

	err := ioutil.WriteFile(fn, []byte(result), 0777)
	if err != nil {
		fmt.Println(err)
	}
}

func DownloadStream(handle string) {
	var mp TweetSuperMap

	dn := fmt.Sprintf("../tweets/%v", handle)
	fn := fmt.Sprintf("../tweets/%v/%v.json", handle, handle)

	os.MkdirAll(dn, 0777)

	if em, err := ioutil.ReadFile(fn); err == nil {
		json.Unmarshal(em, &mp)
	} else {
		mp = map[int64]TweetMap{}
	}

	userTimelineParams := &twitter.UserTimelineParams{
		Count: 2000,
		TweetMode: "extended",
		ScreenName: handle,
		TrimUser: newFalse(),
		ExcludeReplies: newFalse(),
		IncludeRetweets: newFalse(),
	}

	tweets, _, _ := client.Timelines.UserTimeline(userTimelineParams)

	for _, t := range tweets {
		pid := t.InReplyToStatusID
		tid := t.ID
		txt := t.FullText
		dtm, _ := t.CreatedAtTime()

		if pid == 0 {
			pid = tid
		}

		if mp[pid] == nil {
			mp[pid] = map[int64]*Tweet{}
		}

		tweet := NewTweet(tid, dtm, txt)
		if t.ExtendedEntities != nil {
			for _, m := range t.ExtendedEntities.Media {
				if m.Type == "video" {
					var media Media
					for _, v := range m.VideoInfo.Variants {
						if v.Bitrate == 0 {
							continue
						}
						media = NewMedia(m.ID, m.Type, v.URL, v.Bitrate)
					}
					tweet.AddMedia(handle, media)
				} else {
					media := NewMedia(m.ID, m.Type, m.MediaURLHttps, 0)
					tweet.AddMedia(handle, media)
				}
			}
		}
		
		mp[pid][tweet.ID] = tweet

	}

	m, _ := json.MarshalIndent(mp, "", "  ")
	err := ioutil.WriteFile(fn, m, 0777)
	if err != nil {
		fmt.Println(err)
	}

	// for _, tsk := range mp.SortedKeys() {

	narr := [][]*Tweet{}
	keys := mp.SortedKeys()
	for i := len(keys)-1; i >= 0; i-- {
		ts := mp[keys[i]]
		narr1 := []*Tweet{}
		for _, tk := range ts.SortedKeys() {
			t := ts[tk]
			narr1 = append(narr1, t)
		}
		narr = append(narr, narr1)
	}

	Render(handle, narr)
}

func main() {
	wg.Add(1)

	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	ckey    := os.Getenv("API")
	csec    := os.Getenv("SECRET")
	hnd     := os.Getenv("HANDLES")
	
	handles := strings.Split(hnd, ",")

	config := &clientcredentials.Config{
		ClientID:     ckey,
		ClientSecret: csec,
		TokenURL:     "https://api.twitter.com/oauth2/token",
	}

	httpClient := config.Client(oauth2.NoContext)
	client = twitter.NewClient(httpClient)

	for _, h := range handles {
		hh := strings.TrimSpace(h)
		go DownloadStream(hh)
	}
	
	wg.Wait()
}