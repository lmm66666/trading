//go:build integration

package mysql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/port"
)

func chartBoardConfigText(name string) string {
	return `{"defaultSymbol":"SSE:600938","timeframe":"DAY","priceView":"QFQ","indicators":[],"comparison":null,"paneWeights":{},"visibleBars":120,"memo":"` + name + `"}`
}

func TestChartBoardStoreCRUDAndActivation(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			require.True(t, db.Migrator().HasTable(&ChartBoardModel{}))
			ctx := context.Background()
			store := NewChartBoardStore(db)

			// 空表。
			state, err := store.List(ctx)
			require.NoError(t, err)
			require.Empty(t, state.Boards)
			require.Zero(t, state.ActiveID)

			// 创建即激活，新看板成为唯一激活行。
			first, err := store.Create(ctx, "默认看板", chartBoardConfigText("first"))
			require.NoError(t, err)
			require.Len(t, first.Boards, 1)
			require.Equal(t, "默认看板", first.Boards[0].Name)
			require.Equal(t, chartBoardConfigText("first"), first.Boards[0].Config)
			require.Equal(t, first.Boards[0].ID, first.ActiveID)

			second, err := store.Create(ctx, "周线", chartBoardConfigText("second"))
			require.NoError(t, err)
			require.Len(t, second.Boards, 2)
			require.Equal(t, second.Boards[1].ID, second.ActiveID)

			// 列表按 id 升序。
			state, err = store.List(ctx)
			require.NoError(t, err)
			require.Equal(t, second, state)
			require.Less(t, state.Boards[0].ID, state.Boards[1].ID)

			firstID, secondID := state.Boards[0].ID, state.Boards[1].ID

			// 同值更新不能误报 NotFound（MySQL 默认无 CLIENT_FOUND_ROWS）。
			sameName, sameConfig := "周线", chartBoardConfigText("second")
			state, err = store.Update(ctx, secondID, &sameName, &sameConfig)
			require.NoError(t, err)
			require.Len(t, state.Boards, 2)
			require.Equal(t, secondID, state.ActiveID)

			newName, newConfig := "油价", chartBoardConfigText("updated")
			state, err = store.Update(ctx, secondID, &newName, &newConfig)
			require.NoError(t, err)
			require.Equal(t, "油价", state.Boards[1].Name)
			require.Equal(t, chartBoardConfigText("updated"), state.Boards[1].Config)
			require.Equal(t, secondID, state.ActiveID)

			_, err = store.Update(ctx, 999, &newName, nil)
			require.ErrorIs(t, err, port.ErrChartBoardNotFound)

			// 重复激活同一看板保持幂等。
			state, err = store.Activate(ctx, secondID)
			require.NoError(t, err)
			require.Equal(t, secondID, state.ActiveID)
			state, err = store.Activate(ctx, firstID)
			require.NoError(t, err)
			require.Equal(t, firstID, state.ActiveID)
			require.Len(t, state.Boards, 2)
			_, err = store.Activate(ctx, 999)
			require.ErrorIs(t, err, port.ErrChartBoardNotFound)

			// 删除非激活行不影响激活行。
			state, err = store.Delete(ctx, secondID)
			require.NoError(t, err)
			require.Len(t, state.Boards, 1)
			require.Equal(t, firstID, state.ActiveID)

			// 删除激活行后激活剩余 id 最小者。
			third, err := store.Create(ctx, "第三", chartBoardConfigText("third"))
			require.NoError(t, err)
			require.Equal(t, third.Boards[1].ID, third.ActiveID)
			thirdID := third.Boards[1].ID
			state, err = store.Delete(ctx, thirdID)
			require.NoError(t, err)
			require.Equal(t, firstID, state.ActiveID)

			// 存储层允许删到空表；末板保护由应用层负责。
			state, err = store.Delete(ctx, firstID)
			require.NoError(t, err)
			require.Empty(t, state.Boards)
			require.Zero(t, state.ActiveID)

			_, err = store.Delete(ctx, firstID)
			require.ErrorIs(t, err, port.ErrChartBoardNotFound)
		})
	}
}
