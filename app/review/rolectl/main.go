// rolectl 是审核角色的运维脚本（RVW-050）：角色只经运维流程授予或撤销，不提供在线接口。
//
//	DB_REVIEW=... go run ./app/review/rolectl grant -user 1 -roles reviewer,qa -markets US,DE -languages en,de
//	DB_REVIEW=... go run ./app/review/rolectl revoke -user 1
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"esx/app/review/internal/store"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rolectl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rolectl grant|revoke [flags]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	dsnEnv := fs.String("dsn-env", "DB_REVIEW", "environment variable holding the xbh_review DSN")
	user := fs.Int64("user", 0, "target user id")
	actor := fs.Int64("actor", 0, "operator user id recorded in the audit log (0 = ops)")
	roles := fs.String("roles", "", "comma-separated roles: "+strings.Join(store.AllRoles, ","))
	markets := fs.String("markets", "US,DE,ID", "comma-separated markets")
	languages := fs.String("languages", "en,de,id", "comma-separated languages")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	dsn := os.Getenv(*dsnEnv)
	if dsn == "" {
		return fmt.Errorf("%s is not set", *dsnEnv)
	}
	conn, err := sqlx.NewConn(sqlx.SqlConf{DriverName: "mysql", DataSource: dsn})
	if err != nil {
		return err
	}
	if err := util.InitSnowflakeFromEnv(15, 1); err != nil {
		return err
	}
	s := store.New(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch args[0] {
	case "grant":
		err = s.GrantRoles(ctx, *actor, store.Grant{
			UserID: *user, Roles: split(*roles), Markets: split(*markets), Languages: split(*languages),
		}, time.Now())
	case "revoke":
		err = s.RevokeRoles(ctx, *actor, *user, time.Now())
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s user %d ok\n", args[0], *user)
	return nil
}

func split(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
