package module

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/shopspring/decimal"
)

var twOptionCrawlerInstance *TwOptionCrawler
var createTwOptionCrawlerOnce sync.Once

type TwOptionCrawler struct {
	chromedpContext context.Context
	chromedpCancel  context.CancelFunc
}

func GetTwOptionCrawler() *TwOptionCrawler {
	createTwOptionCrawlerOnce.Do(func() {
		twOptionCrawlerInstance = createTwOptionCrawler()
	})
	return twOptionCrawlerInstance
}

func createTwOptionCrawler() *TwOptionCrawler {

	//headless為true的時候, 會看不到畫面
	//start-maximized為true的時候, 可以把瀏覽器放到最大
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), chromedp.Flag("start-maximized", true))

	allocator, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocator)

	return &TwOptionCrawler{
		chromedpContext: ctx,
		chromedpCancel:  cancel,
	}
}

func (vc *TwOptionCrawler) GoToFutureDownloadPage() {
	//https://www.taifex.com.tw/cht/3/dlFutPrevious30DaysSalesData

	chromedp.Run(vc.chromedpContext,
		chromedp.Navigate(`https://www.taifex.com.tw/cht/3/dlFutPrevious30DaysSalesData`),
		chromedp.WaitReady(`#printhere > table:nth-child(4) > tbody`),
	)
}

func (vc *TwOptionCrawler) DoWork() {
	//download
	dateList := vc.download()
	//move to specific folder
	zipFileList := vc.moveZipToTmpFolder(dateList)
	// unzip file
	csvFileList := vc.unzipAndCopy(zipFileList)
	// read csv
	// transform data to k line data
	// save k line data to another file
	vc.readCsvAndTransformToKLines(csvFileList)

}

func (vc *TwOptionCrawler) download() []string {
	dateList := make([]string, 0)
	yAxis := 20
	for row := 2; row <= 31; row++ {
		dateStr := ""
		pubDesc := ""
		if isElementExist(vc.chromedpContext, fmt.Sprint(`#printhere > table:nth-child(4) > tbody > tr:nth-child(`, row, `) > td:nth-child(4) > input`)) {
			chromedp.Run(vc.chromedpContext,
				chromedp.Text(fmt.Sprint(`#printhere > table:nth-child(4) > tbody > tr:nth-child(`, row, `) > td:nth-child(2)`), &dateStr, chromedp.ByQuery),
				chromedp.Text(fmt.Sprint(`#printhere > table:nth-child(4) > tbody > tr:nth-child(`, row, `) > td:nth-child(1)`), &pubDesc, chromedp.ByQuery),
			)

			zipFileName := vc.getZipFileName(dateStr)
			if strings.Contains(pubDesc, "下午") {
				if _, err := os.Stat(fmt.Sprint(`C:\Users\user\Downloads\`, zipFileName)); errors.Is(err, os.ErrNotExist) {
					chromedp.Run(vc.chromedpContext,
						chromedp.Click(fmt.Sprint(`#printhere > table:nth-child(4) > tbody > tr:nth-child(`, row, `) > td:nth-child(4) > input`), chromedp.ByQuery),
						chromedp.Sleep(3*time.Second),
					)
				} else {
					time.Sleep(500 * time.Millisecond)
				}
			} else {
				fmt.Println("資料為 上午 發出, 尚未完整, 不做下載 => ", zipFileName)
			}
		}
		if strings.Contains(pubDesc, "下午") {
			dateList = append(dateList, strings.Trim(dateStr, " "))
		}

		if row%7 == 0 {
			yAxis += 100
			scrollToY(vc.chromedpContext, yAxis)
		}
	}
	return dateList
}

func (vc *TwOptionCrawler) moveZipToTmpFolder(dateList []string) []string {
	zipFileList := make([]string, 0)
	for _, d := range dateList {

		zipFileName := vc.getZipFileName(d)

		if _, err := os.Stat(fmt.Sprint(`./tmpzip/`, zipFileName)); errors.Is(err, os.ErrNotExist) {
			source, err := os.Open(fmt.Sprint(`C:\Users\user\Downloads\`, zipFileName))
			handleError(err)
			dest, err := os.Create(fmt.Sprint(`./tmpzip/`, zipFileName))
			handleError(err)
			_, err = io.Copy(dest, source)
			handleError(err)
			dest.Close()
			source.Close()
		}

		zipFileList = append(zipFileList, zipFileName)
	}
	return zipFileList
}

func (vc *TwOptionCrawler) unzipAndCopy(zipFileList []string) []string {
	csvFileList := make([]string, 0)
	for _, z := range zipFileList {
		//fmt.Println("zip file=>", z)
		archive, err := zip.OpenReader(fmt.Sprint(`./tmpzip/`, z))
		handleError(err)
		for _, f := range archive.File {
			fileInArchive, err := f.Open()
			handleError(err)

			if _, err := os.Stat(fmt.Sprint(`./tmpcsv/`, f.Name)); errors.Is(err, os.ErrNotExist) {
				dest, err := os.Create(fmt.Sprint(`./tmpcsv/`, f.Name))
				handleError(err)
				_, err = io.Copy(dest, fileInArchive)
				handleError(err)
				dest.Close()
			}

			fileInArchive.Close()

			csvFileList = append(csvFileList, f.Name)
		}

		archive.Close()
	}
	return csvFileList
}

func (vc *TwOptionCrawler) readCsvAndTransformToKLines(csvFileList []string) {
	for _, csv := range csvFileList {
		f, err := os.Open(fmt.Sprint(`./tmpcsv/`, csv))
		handleError(err)
		b := new(strings.Builder)
		io.Copy(b, f)
		lines := strings.Split(b.String(), "\n")
		symbolToDataList := make(map[string][]*TickData)
		isHead := true
		for _, l := range lines {
			if isHead {
				isHead = false
				continue
			}
			if strings.Trim(l, " ") == "" {
				continue
			}
			rawFields := strings.Split(l, ",")
			if strings.Trim(rawFields[6], " ") != "-" || strings.Trim(rawFields[7], " ") != "-" {
				continue
			}

			symbol := strings.Trim(rawFields[1], " ")
			if symbol == "TX" || symbol == "MTX" {
				settleMonth := strings.Trim(rawFields[2], " ")
				key := fmt.Sprint(symbol, "_", settleMonth)
				val, ok := symbolToDataList[key]
				if !ok {
					val = make([]*TickData, 0)
				}
				price, err := decimal.NewFromString(strings.Trim(rawFields[4], " "))
				handleError(err)
				qty, err := decimal.NewFromString(strings.Trim(rawFields[5], " "))
				handleError(err)
				val = append(val, &TickData{
					Symbol:      symbol,
					SettleMonth: settleMonth,
					TradeDate:   strings.Trim(rawFields[0], " "),
					DealTime:    strings.Trim(rawFields[3], " "),
					Price:       price,
					Qty:         qty.Div(decimal.NewFromInt(2)),
				})
				symbolToDataList[key] = val
			}

		}

		for kk, vv := range symbolToDataList {
			dateSs := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(csv, "Daily", ""), ".csv", ""), "_", "")
			fileName := fmt.Sprint(kk, "_", dateSs, "_1min.txt")

			if _, err := os.Stat(fmt.Sprint(`./klines/`, fileName)); errors.Is(err, os.ErrNotExist) {
				buf := vc.transformToKLinesFileBytes(vv)

				dest, err := os.Create(fmt.Sprint(`./klines/`, fileName))
				handleError(err)
				_, err = io.Copy(dest, strings.NewReader(buf.String()))
				handleError(err)
				dest.Close()
			}
		}

		b.Reset()
		f.Close()
	}
}

func (vc *TwOptionCrawler) transformToKLinesFileBytes(tickRows []*TickData) *strings.Builder {

	kLineList := make([]*KLine, 0)

	st, err := time.ParseInLocation("20060102", tickRows[0].TradeDate, time.Local)
	handleError(err)
	hour, err := strconv.Atoi(tickRows[0].DealTime[0:2])
	handleError(err)
	min, err := strconv.Atoi(tickRows[0].DealTime[2:4])
	handleError(err)

	st = st.Add(time.Duration(hour) * time.Hour)
	st = st.Add(time.Duration(min) * time.Minute)

	et := st.Add(1 * time.Minute)

	var currentKLine *KLine
	for _, r := range tickRows {
		tradeTime := vc.parseLineDateTime(r.TradeDate, r.DealTime)
		//fmt.Println("tradeTime=", tradeTime, " st=", st, " et=", et)

		if !tradeTime.Before(st) && tradeTime.Before(et) {
			if currentKLine == nil {
				currentKLine = &KLine{
					TimeStart: st,
					TimeEnd:   et,
					Symbol:    r.Symbol,
					Open:      r.Price,
					High:      r.Price,
					Low:       r.Price,
					Close:     r.Price,
					Qty:       r.Qty,
				}
			} else {
				if r.Price.GreaterThan(currentKLine.High) {
					currentKLine.High = r.Price
				}
				if r.Price.LessThan(currentKLine.Low) {
					currentKLine.Low = r.Price
				}
				currentKLine.Close = r.Price
				currentKLine.Qty = currentKLine.Qty.Add(r.Qty)
			}
		}

		if !tradeTime.Before(et) { // tradeTime >= endtime
			//fmt.Println(" append=", currentKLine)
			kLineList = append(kLineList, currentKLine)
			year, month, day := tradeTime.Date()
			if year != et.Year() || month != et.Month() || day != et.Day() { // 如果日期不同, 代表換日
				st = vc.parseLineDateTime(r.TradeDate, r.DealTime)
				et = st.Add(1 * time.Minute)
			} else {
				st = time.Date(tradeTime.Year(), tradeTime.Month(), tradeTime.Day(), tradeTime.Hour(), tradeTime.Minute(), 0, 0, time.Local)
				et = st.Add(1 * time.Minute) //通常是+1分鐘
			}
			//fmt.Println("=> tradeTime=", tradeTime, " st=", st, " et=", et)
			if !tradeTime.Before(st) && tradeTime.Before(et) {
				currentKLine = &KLine{
					TimeStart: st,
					TimeEnd:   et,
					Symbol:    r.Symbol,
					Open:      r.Price,
					High:      r.Price,
					Low:       r.Price,
					Close:     r.Price,
					Qty:       r.Qty,
				}
				//fmt.Println("=> append=", currentKLine)
				//kLineList = append(kLineList, currentKLine)
			}

		}
	}
	if currentKLine != nil {
		kLineList = append(kLineList, currentKLine)
		//fmt.Println("## append=", currentKLine)
	} else {
		//fmt.Println("XX append=", currentKLine)
	}

	buf := new(strings.Builder)
	for _, k := range kLineList {
		if k == nil {
			//fmt.Println("k is nil")
			continue
		}
		o, _ := k.Open.Float64()
		h, _ := k.High.Float64()
		ll, _ := k.Low.Float64()
		c, _ := k.Close.Float64()
		q, _ := k.Qty.Float64()
		buf.WriteString(fmt.Sprint(k.TimeEnd.Format("2006-01-02T15:04:05"), ",", o, ",", h, ",", ll, ",", c, ",", q, "\n"))
	}
	return buf
}

func (vc *TwOptionCrawler) parseLineDateTime(date string, timePart string) time.Time {
	st, err := time.ParseInLocation("20060102", date, time.Local)
	handleError(err)
	hour, err := strconv.Atoi(timePart[0:2])
	handleError(err)
	min, err := strconv.Atoi(timePart[2:4])
	handleError(err)
	sec, err := strconv.Atoi(timePart[4:])
	handleError(err)
	st = st.Add(time.Duration(hour) * time.Hour)
	st = st.Add(time.Duration(min) * time.Minute)
	st = st.Add(time.Duration(sec) * time.Second)
	return st
}

func (vc *TwOptionCrawler) getZipFileName(dateStr string) string {
	newDateStr := strings.ReplaceAll(dateStr, "/", "_")
	zipFileName := fmt.Sprint(`Daily_`, newDateStr, `.zip`)
	return zipFileName
}

func (vc *TwOptionCrawler) Release() {
	defer vc.chromedpCancel()
}

type TickData struct {
	TradeDate   string
	Symbol      string
	SettleMonth string
	DealTime    string
	Price       decimal.Decimal
	Qty         decimal.Decimal
}

type KLine struct {
	TimeStart time.Time
	TimeEnd   time.Time //not include
	Symbol    string
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Qty       decimal.Decimal
}
