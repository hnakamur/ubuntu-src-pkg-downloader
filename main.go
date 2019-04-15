package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
)

func main() {
	destDir := flag.String("dest", "", "destination directory")
	suite := flag.String("suite", "disco", "suite name")
	pkg := flag.String("pkg", "", "source package name")
	baseURL := flag.String("base-url", "https://packages.ubuntu.com", "base URL of package page")
	archiveURLPrefix := flag.String("archive-url-prefix", "http://archive.ubuntu.com/", "archive URL prefix")
	timeout := flag.Duration("timeout", time.Minute, "http client timeout")
	flag.Parse()

	packagePageURL, err := getPackagePageURL(*baseURL, *suite, *pkg)
	if err != nil {
		log.Fatal(err)
	}
	fileURLs, err := getFileURLs(packagePageURL, *archiveURLPrefix)
	if err != nil {
		log.Fatal(err)
	}

	err = downloadFiles(fileURLs, *timeout, *destDir)
	if err != nil {
		log.Fatal(err)
	}
}

func getPackagePageURL(baseURL, suite, pkg string) (*url.URL, error) {
	return url.Parse(fmt.Sprintf("%s/%s/%s", baseURL, suite, pkg))
}

func getFileURLs(packagePageURL *url.URL, archiveURLPrefix string) ([]string, error) {
	doc, err := goquery.NewDocument(packagePageURL.String())
	if err != nil {
		return nil, err
	}
	var fileURLs []string
	doc.Find("#pmoreinfo > ul > li > a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		if strings.HasPrefix(href, archiveURLPrefix) {
			fileURLs = append(fileURLs, href)
		}
	})
	return fileURLs, nil
}

func downloadFiles(fileURLs []string, timeout time.Duration, destDir string) error {
	if destDir == "" {
		var err error
		destDir, err = ioutil.TempDir("", "ubuntu-src-pkg")
		if err != nil {
			return err
		}
	} else {
		err := os.MkdirAll(destDir, 0700)
		if err != nil {
			return err
		}
	}

	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg))

	wg.Add(len(fileURLs))
	for _, fileURL := range fileURLs {
		fileURL := fileURL

		go func() {
			defer wg.Done()

			client := http.Client{Timeout: timeout}
			base := path.Base(fileURL)
			destFile := filepath.Join(destDir, base)
			file, err := os.Create(destFile)
			if err != nil {
				log.Println(err)
			}
			defer file.Close()

			resp, err := client.Get(fileURL)
			if err != nil {
				log.Println(err)
			}
			defer resp.Body.Close()

			contentLength := resp.ContentLength

			bar := p.AddBar(contentLength,
				mpb.PrependDecorators(
					decor.Name(base),
				),
				mpb.AppendDecorators(
					decor.Percentage(decor.WCSyncSpace),
				),
			)

			_, err = io.Copy(file, bar.ProxyReader(resp.Body))
			if err != nil {
				log.Println(err)
			}
		}()
	}
	p.Wait()
	log.Printf("downloaded files to %s", destDir)
	return nil
}
