package main

import (
	"bytes"
	"encoding/csv"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

func main() {
	logger := log.New(os.Stdout, "INFO: ", log.Lshortfile)

	//Host folder to retrieve audio
	domainName := os.Getenv("DOMAIN_NAME")
	fs := http.FileServer(http.Dir("./audios"))
	go http.ListenAndServe(":9000", fs)

	//Start up bot
	pref := tele.Settings{
		Token:  os.Getenv("TOKEN"),
		Poller: &tele.LongPoller{Timeout: 60 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal(err)
		return
	}

	b.Handle(tele.OnQuery, func(c tele.Context) error {
		return sendMessage(logger, c, domainName)
	})

	b.Start()
}

func isWhitelisted(logger *log.Logger, name string) bool {
	f, err := os.OpenFile("whitelist.txt", os.O_RDONLY, 0777)
	defer f.Close()

	if err != nil {
		logger.Println(err)
	}
	byteLines, err := ioutil.ReadAll(f)
	if err != nil {
		logger.Println(err)
	}
	for _, v := range bytes.Split(byteLines, []byte("\n")) {
		if name == string(v) {
			return true
		}
	}
	return false
}

// isHosted searches in file for name and returns link (file: link, name)
func isHosted(logger *log.Logger, link string) (string, bool) {
	f, err := os.OpenFile("hosted.txt", os.O_RDONLY, 0777)
	if err != nil {
		return "", false
	}
	defer f.Close()

	reader := csv.NewReader(f)
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if record[0] == link {
			logger.Println("Got from cache, ", record[1])
			return record[1], true
		}
	}
	return "", false
}

func downloadAudio(logger *log.Logger, link string) {
	cmdd := exec.Command("yt-dlp",
		"-x",
		"--audio-format", "mp3",
		"-o", "audios/%(title)s.%(ext)s",
		"--audio-quality", "128K",
		"--no-check-certificate",
		"--no-playlist",
		link)
	//cmdd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmdd.Run(); err != nil {
		logger.Println(err)
	}
}

func downloadName(logger *log.Logger, link string) string {
	var name string
	cmdn, err := exec.Command("yt-dlp",
		"--print", "filename",
		"-o", "%(title)s",
		"--no-playlist",
		link).Output()
	if err != nil {
		logger.Println(err)
	}

	//cut newline from stdout to get name
	if len(cmdn) > 1 {
		name = string(cmdn[0 : len(cmdn)-1])
	}
	logger.Println(name)
	return name
}

func sendMessage(logger *log.Logger, c tele.Context, domainName string) error {
	query := c.Query()
	link := query.Text
	who := query.Sender.Username
	if !isWhitelisted(logger, who) {
		return nil
	}
	name := "not found"
	hostedName, hosted := isHosted(logger, link)
	//check if i need to download or if file already present
	if !hosted {
		//Download song
		downloadAudio(logger, link)
	}
	//check if already hosted otherwise add name to csv file
	if hosted {
		name = hostedName
	} else {
		//Download name
		name = downloadName(logger, link)

		//write to disk to get someday if accessed again
		f, err := os.OpenFile("hosted.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
		if err != nil {
			logger.Println(err)
		}
		writer := csv.NewWriter(f)
		writer.Write([]string{link, name})
		writer.Flush()
		err = f.Close()
		if err != nil {
			logger.Println(err)
		}
	}

	//Respond
	linkToHosted := strings.ReplaceAll(domainName+name+".mp3", " ", "%20")
	results := make(tele.Results, 1)
	result := &tele.AudioResult{
		Title: name,
		URL:   linkToHosted,
	}
	result.SetResultID(strconv.Itoa(1))
	results[0] = result
	c.Answer(&tele.QueryResponse{
		Results:    results,
		IsPersonal: true,
		CacheTime:  10,
	})

	if !hosted {
		c.Send("Loaded")
	}

	return nil
}
