//go:build integration

package mysql

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
)

func TestOpaqueIdentityIsExactOnMySQL84(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			for _, entry := range exactIdentityModels {
				s, err := schema.Parse(entry.model, &sync.Map{}, schema.NamingStrategy{})
				require.NoError(t, err)
				for name := range entry.fields {
					t.Run(s.Table+"/"+name, func(t *testing.T) {
						tx := db.Begin()
						require.NoError(t, tx.Error)
						defer tx.Rollback()
						// Keep every other member of a UNIQUE index containing this
						// column constant, so only this key can distinguish the rows.
						shared := map[string]bool{}
						for _, index := range s.ParseIndexes() {
							if index.Class != "UNIQUE" {
								continue
							}
							contains := false
							for _, field := range index.Fields {
								if field.Name == name {
									contains = true
								}
							}
							if contains {
								for _, field := range index.Fields {
									shared[field.Name] = true
								}
							}
						}
						for i, key := range []string{"Key", "key", "key "} {
							value := reflect.New(reflect.TypeOf(entry.model).Elem())
							elem := value.Elem()
							for _, field := range s.Fields {
								if field.PrimaryKey || field.Name == "CreatedAt" || field.Name == "UpdatedAt" {
									continue
								}
								f := elem.FieldByName(field.Name)
								switch f.Kind() {
								case reflect.String:
									f.SetString(fmt.Sprintf("value-%d", i))
								case reflect.Uint64, reflect.Uint32:
									f.SetUint(uint64(i + 100))
								case reflect.Int64:
									f.SetInt(1)
								case reflect.Slice:
									if f.Type().Elem().Kind() == reflect.Uint8 {
										f.SetBytes([]byte("{}"))
									}
								case reflect.Struct:
									if f.Type() == reflect.TypeOf(time.Time{}) {
										f.Set(reflect.ValueOf(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
									}
								}
								if field.NotNull && field.Name == "Code" {
									f.SetString(fmt.Sprintf("%06d", i))
								}
							}
							for fieldName := range shared {
								f := elem.FieldByName(fieldName)
								switch f.Kind() {
								case reflect.String:
									f.SetString("shared")
								case reflect.Uint64, reflect.Uint32:
									f.SetUint(1)
								}
							}
							elem.FieldByName(name).SetString(key)
							require.NoError(t, tx.Create(value.Interface()).Error)
						}
						for _, key := range []string{"Key", "key", "key "} {
							var count int64
							require.NoError(t, tx.Model(entry.model).Where(s.FieldsByName[name].DBName+" = ?", key).Count(&count).Error)
							require.Equal(t, int64(1), count, name+"="+key)
						}
					})
				}
			}
		})
	}
}

func TestUnboundedIdentityBytesRoundTripOnMySQL84(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for i, key := range []string{"Key", "key", "key ", "成交标识 ", strings.Repeat("长", 500)} {
				order := BacktestOrderModel{RunID: "run", Sequence: uint64(i + 1), OrderID: key, CreatedTime: at}
				require.NoError(t, db.Create(&order).Error)
				trade := BacktestTradeModel{RunID: "run", Sequence: uint64(i + 1), OrderID: key, FillID: key, Time: at}
				require.NoError(t, db.Create(&trade).Error)
				var stored BacktestOrderModel
				require.NoError(t, db.Where("run_id = ? AND sequence = ?", "run", i+1).Take(&stored).Error)
				require.Equal(t, []byte(key), []byte(stored.OrderID))
				var fill BacktestTradeModel
				require.NoError(t, db.Where("run_id = ? AND sequence = ?", "run", i+1).Take(&fill).Error)
				require.Equal(t, key, fill.OrderID)
				require.Equal(t, key, fill.FillID)
			}
		})
	}
}
