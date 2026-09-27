package preview

// The real screenshot driver: chromedp driving the person's own Chrome or Edge. It is never launched
// by a test - the service takes a Shooter, and every test gives a fake - so no machine has to have a
// browser installed to run this module's tests, and the owner's machine is never driven by a build.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chromedp/chromedp"
)

// chromeShooter takes a screenshot by driving the browser at req.Browser. It runs headless, at the
// size it was asked for, in the card's own profile directory, so two cards' shots cannot share
// cookies or state.
type chromeShooter struct{}

// Shoot opens the browser, loads the page, waits for it to have a body, writes the image, and closes
// the browser again. A page that never settles is bounded by the caller's context.
func (chromeShooter) Shoot(ctx context.Context, req ShotRequest) (Shot, error) {
	if req.Browser == "" {
		return Shot{}, errors.New("preview: no browser to drive")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return Shot{}, fmt.Errorf("preview: a screenshot needs a size, got %dx%d", req.Width, req.Height)
	}
	if err := os.MkdirAll(req.ProfileDir, 0o700); err != nil {
		return Shot{}, err
	}
	if err := os.MkdirAll(filepath.Dir(req.Path), 0o700); err != nil {
		return Shot{}, err
	}
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(req.Browser),
		chromedp.UserDataDir(req.ProfileDir),
		chromedp.Flag("headless", true),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(req.Width, req.Height),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, options...)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	var image []byte
	if err := chromedp.Run(taskCtx,
		chromedp.EmulateViewport(int64(req.Width), int64(req.Height)),
		chromedp.Navigate(req.URL),
		chromedp.WaitReady("body"),
		chromedp.CaptureScreenshot(&image),
	); err != nil {
		return Shot{}, err
	}
	if err := os.WriteFile(req.Path, image, 0o600); err != nil {
		return Shot{}, err
	}
	return Shot{Width: req.Width, Height: req.Height}, nil
}
