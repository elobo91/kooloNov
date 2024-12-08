package action

import (
	"slices"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/d2go/pkg/data/npc"
	"github.com/hectorgimenez/koolo/internal/action/step"
	"github.com/hectorgimenez/koolo/internal/context"
	"github.com/hectorgimenez/koolo/internal/utils"
)

func OpenTPIfLeader() error {
	ctx := context.Get()
	ctx.SetLastAction("OpenTPIfLeader")

	isLeader := ctx.CharacterCfg.Companion.Leader

	if isLeader {
		return step.OpenPortal()
	}

	return nil
}

func IsMonsterSealElite(monster data.Monster) bool {
	return monster.Type == data.MonsterTypeSuperUnique && (monster.Name == npc.OblivionKnight || monster.Name == npc.VenomLord || monster.Name == npc.StormCaster)
}

func PostRun(isLastRun bool) error {
	ctx := context.Get()
	ctx.SetLastAction("PostRun")

	// Allow some time for items drop to the ground, otherwise we might miss some
	utils.Sleep(200)
	ClearAreaAroundPlayer(5, data.MonsterAnyFilter())
	ItemPickup(-1)

	// Don't return town on last run
	if !isLastRun {
		return ReturnTown()
	}

	return nil
}
func AreaCorrection() error {
	ctx := context.Get()
	currentArea := ctx.Data.PlayerUnit.Area
	expectedArea := ctx.CurrentGame.AreaCorrection.ExpectedArea

	// Skip correction if in town, if we're in the expected area, or if expected area is not set
	if currentArea.IsTown() || currentArea == expectedArea || expectedArea == 0 {
		return nil
	}

	// Define pairs of areas that are openly connected (no entrance/portal required)
	openAreaPairs := map[area.ID][]area.ID{
		area.FrigidHighlands: {area.BloodyFoothills, area.ArreatPlateau},
		area.BloodyFoothills: {area.FrigidHighlands},
		area.ArreatPlateau:   {area.FrigidHighlands},
		// We only need to add open areas where we use movetocoords to change area like eldritch
	}

	// Skip correction if we're between openly connected areas
	if connectedAreas, exists := openAreaPairs[expectedArea]; exists {
		if slices.Contains(connectedAreas, currentArea) {
			return nil
		}
	}

	// Only correct if area correction is enabled and we're in an unexpected area
	if ctx.CurrentGame.AreaCorrection.Enabled && ctx.CurrentGame.AreaCorrection.ExpectedArea != ctx.Data.AreaData.Area {
		ctx.Logger.Info("Accidentally went to adjacent area, returning to expected area",
			"current", ctx.Data.AreaData.Area.Area().Name,
			"expected", ctx.CurrentGame.AreaCorrection.ExpectedArea.Area().Name)
		return MoveToArea(ctx.CurrentGame.AreaCorrection.ExpectedArea)
	}

	return nil
}
