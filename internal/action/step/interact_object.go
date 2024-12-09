package step

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/mode"
	"github.com/hectorgimenez/d2go/pkg/data/state"
	"github.com/hectorgimenez/koolo/internal/context"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/ui"
	"github.com/hectorgimenez/koolo/internal/utils"
)

const (
	maxInteractionAttempts = 5
	portalSyncDelay        = 200
	maxPortalSyncAttempts  = 15
)

func InteractObject(obj data.Object, isCompletedFn func() bool) error {
	ctx := context.Get()
	ctx.SetLastStep("InteractObject")

	interactionAttempts := 0
	mouseOverAttempts := 0
	waitingForInteraction := false
	currentMouseCoords := data.Position{}
	lastHoverCoords := data.Position{}
	lastRun := time.Now()

	// If no completion check provided and not defined here default to waiting for interaction
	if isCompletedFn == nil {
		isCompletedFn = func() bool {
			//For stash if we have open menu we can return early
			if strings.EqualFold(string(obj.Name), "Bank") {
				return ctx.Data.OpenMenus.Stash
			}
			if obj.IsChest() {
				chest, found := ctx.Data.Objects.FindByID(obj.ID)
				// Since opening a chest is immediate and the mode changes right away,
				// we can return true as soon as we see these states
				if !found || chest.Mode == mode.ObjectModeOperating || chest.Mode == mode.ObjectModeOpened {
					return true
				}
				// Also return true if no longer selectable (as a fallback)
				return !chest.Selectable
			}
			return waitingForInteraction
		}
	}
	// State JustPortaled is instant, when detected we can consider it completed.
	if obj.IsPortal() || obj.IsRedPortal() {
		isCompletedFn = func() bool {
			return ctx.Data.PlayerUnit.States.HasState(state.JustPortaled)
		}
	}

	for !isCompletedFn() {
		ctx.PauseIfNotPriority()

		if interactionAttempts >= maxInteractionAttempts || mouseOverAttempts >= 20 {
			return fmt.Errorf("failed interacting with object")
		}

		ctx.RefreshGameData()
		// Give some time before retrying the interaction
		if waitingForInteraction && time.Since(lastRun) < time.Millisecond*200 {
			//for chest we can check more often status is almost instant
			if !obj.IsChest() || time.Since(lastRun) < time.Millisecond*50 {
				continue
			}
		}

		var o data.Object
		var found bool
		if obj.ID != 0 {
			o, found = ctx.Data.Objects.FindByID(obj.ID)
		} else {
			o, found = ctx.Data.Objects.FindOne(obj.Name)
		}
		if !found {
			return fmt.Errorf("object %v not found", obj)
		}

		lastRun = time.Now()

		// If portal is still being created, wait
		if o.IsPortal() || o.IsRedPortal() {
			if o.Mode == mode.ObjectModeOperating || o.Mode != mode.ObjectModeOpened {
				utils.Sleep(100)
				continue
			}
		}

		if o.IsChest() && o.Mode == mode.ObjectModeOperating {
			continue // Skip if chest is already being opened
		}
		//if we hovered the portal once then we have exact hitbox, lets store and reuse it
		if o.IsHovered && currentMouseCoords != (data.Position{}) && (o.IsPortal() || o.IsRedPortal()) {
			lastHoverCoords = currentMouseCoords
		}

		if o.IsHovered {
			ctx.HID.Click(game.LeftButton, currentMouseCoords.X, currentMouseCoords.Y)
			waitingForInteraction = true
			interactionAttempts++

			if (o.IsPortal() || o.IsRedPortal()) && o.PortalData.DestArea != 0 {
				startTime := time.Now()
				for time.Since(startTime) < time.Second*2 {
					// Check for loading screen during portal transition
					if ctx.Data.OpenMenus.LoadingScreen {
						ctx.WaitForGameToLoad()
						break
					}
					if ctx.Data.PlayerUnit.States.HasState(state.JustPortaled) {
						break
					}
					utils.Sleep(50)
				}

				utils.Sleep(500)
				for attempts := 0; attempts < maxPortalSyncAttempts; attempts++ {
					if ctx.Data.PlayerUnit.Area == o.PortalData.DestArea {
						if areaData, ok := ctx.Data.Areas[o.PortalData.DestArea]; ok {
							if areaData.IsInside(ctx.Data.PlayerUnit.Position) {
								if o.PortalData.DestArea.IsTown() {
									return nil
								}
								// For special areas, ensure we have proper object data loaded
								if len(ctx.Data.Objects) > 0 {
									return nil
								}
							}
						}
					}
					utils.Sleep(portalSyncDelay)
				}
				return fmt.Errorf("portal sync timeout - expected area: %v, current: %v", o.PortalData.DestArea, ctx.Data.PlayerUnit.Area)
			}
			continue
		}

		distance := ctx.PathFinder.DistanceFromMe(o.Position)
		if distance > 15 {
			return fmt.Errorf("object is too far away: %d. Current distance: %d", o.Name, distance)
		}

		if mouseOverAttempts == 0 && lastHoverCoords != (data.Position{}) && (o.IsPortal() || o.IsRedPortal()) {
			currentMouseCoords = lastHoverCoords
			ctx.HID.MovePointer(lastHoverCoords.X, lastHoverCoords.Y)
			mouseOverAttempts++
			utils.Sleep(100)
			continue
		}

		objectX := o.Position.X
		objectY := o.Position.Y

		if o.IsPortal() || o.IsRedPortal() {
			mX, mY := ui.GameCoordsToScreenCords(objectX, objectY)
			x, y := portalSpiral(mouseOverAttempts)
			currentMouseCoords = data.Position{X: mX + x, Y: mY + y}
		} else {
			objectX -= 2
			objectY -= 2
			mX, mY := ui.GameCoordsToScreenCords(objectX, objectY)
			x, y := utils.Spiral(mouseOverAttempts)
			x = x / 3
			y = y / 3
			currentMouseCoords = data.Position{X: mX + x, Y: mY + y}
		}

		ctx.HID.MovePointer(currentMouseCoords.X, currentMouseCoords.Y)
		mouseOverAttempts++
		utils.Sleep(100)
	}

	if (obj.IsPortal() || obj.IsRedPortal()) && ctx.Data.PlayerUnit.Area == obj.PortalData.DestArea {
		if areaData, ok := ctx.Data.Areas[obj.PortalData.DestArea]; ok {
			if areaData.IsInside(ctx.Data.PlayerUnit.Position) {
				ctx.CurrentGame.AreaCorrection.Enabled = true
				ctx.CurrentGame.AreaCorrection.ExpectedArea = obj.PortalData.DestArea
				return nil
			}
		}
	}

	return nil
}

// From object.txt portal : sizeX 18 sizeY 19 Left -40 Top -100 Width 80 Height 110 Yoffset 120 Xoffset 120
// TODO We could extract it directly from txt file and make it generic for other objects like entrances, stash, items, corpse.
func portalSpiral(attempt int) (x, y int) {
	baseRadius := float64(attempt) * 3.0
	angle := float64(attempt) * math.Pi * (3.0 - math.Sqrt(5.0))

	xScale := 1.0
	yScale := 110.0 / 80.0

	x = int(baseRadius * math.Cos(angle) * xScale)
	y = int(baseRadius*math.Sin(angle)*yScale) - 50

	x = utils.Clamp(x, -40, 40)
	y = utils.Clamp(y, -100, 10)

	return x, y
}
