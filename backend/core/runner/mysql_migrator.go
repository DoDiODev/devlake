/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runner

import (
	"fmt"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// devlakeMysqlDialector wraps the standard MySQL dialector so we can override
// its migrator.
//
// gorm.io/driver/mysql >= v1.6.0 changed Migrator.AddColumn to always append
// "ADD PRIMARY KEY (col)" whenever the added column is tagged primaryKey. Many
// historical DevLake migrations add a primary-key column to a table that already
// has a primary key (the final composite key is rebuilt afterwards, e.g. via
// migrationhelper.TransformTable). Under the new driver behaviour those
// migrations fail with "Error 1068: Multiple primary key defined".
//
// This wrapper restores the previous, migration-friendly behaviour: a PRIMARY
// KEY clause is only emitted when the target table does not already have one.
type devlakeMysqlDialector struct {
	gorm.Dialector
}

func wrapMysqlDialector(d gorm.Dialector) gorm.Dialector {
	return &devlakeMysqlDialector{Dialector: d}
}

func (d *devlakeMysqlDialector) Migrator(db *gorm.DB) gorm.Migrator {
	base := d.Dialector.Migrator(db)
	mysqlMigrator, ok := base.(mysql.Migrator)
	if !ok {
		return base
	}
	return devlakeMysqlMigrator{Migrator: mysqlMigrator}
}

type devlakeMysqlMigrator struct {
	mysql.Migrator
}

// AddColumn mirrors gorm.io/driver/mysql Migrator.AddColumn but only adds a
// PRIMARY KEY clause when the target table does not already have one.
func (m devlakeMysqlMigrator) AddColumn(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema == nil {
			return fmt.Errorf("failed to get schema")
		}
		f := stmt.Schema.LookUpField(name)
		if f == nil {
			return fmt.Errorf("failed to look up field with name: %s", name)
		}
		if f.IgnoreMigration {
			return nil
		}

		fieldType := m.FullDataTypeOf(f)
		columnName := clause.Column{Name: f.DBName}
		values := []interface{}{m.CurrentTable(stmt), columnName, fieldType}

		var alterSQL strings.Builder
		_, _ = alterSQL.WriteString("ALTER TABLE ? ADD ? ?")

		// gorm.io/driver/mysql v1.5.x (the version DevLake's migrations were
		// written against) never emitted a PRIMARY KEY clause from AddColumn:
		// primary keys are established by CreateTable on fresh tables or by
		// explicit migration logic. v1.6.0 started appending "ADD PRIMARY KEY"
		// for any primaryKey-tagged column, which breaks migrations that add a
		// primary-key column to an existing table or rebuild the key in steps
		// (Error 1068: Multiple primary key defined). Restore the old behaviour:
		// do not add a primary key for plain primaryKey columns. The only case
		// that genuinely requires a key is auto_increment (MySQL rejects an
		// auto_increment column that is not a key), and only when the table does
		// not already have a primary key.
		if strings.Contains(strings.ToLower(fieldType.SQL), "auto_increment") && !m.tableHasPrimaryKey(stmt.Table) {
			_, _ = alterSQL.WriteString(", ADD PRIMARY KEY (?)")
			values = append(values, columnName)
		}
		return m.DB.Exec(alterSQL.String(), values...).Error
	})
}

func (m devlakeMysqlMigrator) tableHasPrimaryKey(table string) bool {
	if table == "" {
		return false
	}
	var count int64
	err := m.DB.Raw(
		"SELECT COUNT(*) FROM information_schema.table_constraints "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND constraint_type = 'PRIMARY KEY'",
		table,
	).Scan(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}
