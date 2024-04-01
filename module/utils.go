package module

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

func isElementExist(ctx context.Context, elemt string) bool {
	exist := false
	err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(fmt.Sprint(`document.querySelector("`, elemt, `") != null`), &exist))
	handleError(err)
	return exist
}

func handleError(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func scrollToY(ctx context.Context, y int) {
	var res string
	chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(fmt.Sprint(`window.scrollTo(0,`, y, `)`), &res))

	fmt.Println(res)
}
