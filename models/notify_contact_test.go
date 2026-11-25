package models

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// The Bark device key contact works on a fresh install without writing the admin-maintained list,
// and once that list holds it, the stored entry (e.g. hidden by an admin) wins
func TestNotifyContactsGetMergesBuiltinContacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Configs{}); err != nil {
		t.Fatal(err)
	}
	c := ctx.NewContext(context.Background(), db, true)

	got, err := NotifyContactsGet(c)
	if err != nil || len(got) != 1 || got[0].Ident != BarkKey || !got[0].BuiltIn {
		t.Fatalf("fresh install should offer the bark contact: %+v, %v", got, err)
	}
	if cval, _ := ConfigsGet(c, NOTIFYCONTACT); cval != "" {
		t.Fatalf("reading must not write the contact list: %q", cval)
	}

	if err := ConfigsSet(c, NOTIFYCONTACT, `[{"name":"Slack ID","ident":"slack_id"}]`); err != nil {
		t.Fatal(err)
	}
	got, _ = NotifyContactsGet(c)
	if len(got) != 2 || got[0].Ident != "slack_id" || got[1].Ident != BarkKey {
		t.Fatalf("custom contacts must be kept and bark appended: %+v", got)
	}

	if err := ConfigsSet(c, NOTIFYCONTACT, `[{"name":"Bark","ident":"bark_device_key","hide":true,"built_in":true}]`); err != nil {
		t.Fatal(err)
	}
	got, _ = NotifyContactsGet(c)
	if len(got) != 1 || !got[0].Hide || got[0].Name != "Bark" {
		t.Fatalf("a stored bark entry must not be duplicated or overridden: %+v", got)
	}
}
