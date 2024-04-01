package main

import (
	"time"
	"tw_data_downloader/configure"
	"tw_data_downloader/env"
	"tw_data_downloader/module"
)

func main() {
	//load configuration
	configure.LoadConfiguration()

	// command
	// env init
	env.Initialize()

	optionDownloader := module.GetTwOptionCrawler()
	defer optionDownloader.Release()
	optionDownloader.GoToFutureDownloadPage()
	optionDownloader.DoWork()

	time.Sleep(10 * time.Second)
}
