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

package migrationscripts

import (
	"github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/migrationscripts/archived"
	"github.com/apache/incubator-devlake/helpers/migrationhelper"
)

// teambitionScopeConfig20260727 mirrors models.TeambitionScopeConfig. The
// migration that created `_tool_teambition_scope_configs` did not include the
// columns of the embedded common.Model (`id`, `created_at`, `updated_at`),
// which the runtime model expects.
//
// Adding the AUTO_INCREMENT primary key `id` to the already existing (and
// primary-key-less) table works: GORM issues a plain ADD COLUMN and MySQL
// backfills consecutive ids for existing rows — verified against MySQL 8.4.
// The `uniqueIndex` on `name` is likewise safe, because newly added columns are
// nullable and both MySQL and PostgreSQL permit duplicate NULLs in a unique
// index.
type teambitionScopeConfig20260727 struct {
	archived.Model
	Entities          []string          `gorm:"type:json;serializer:json" json:"entities"`
	ConnectionId      uint64            `json:"connectionId" gorm:"index"`
	Name              string            `json:"name" gorm:"type:varchar(255);uniqueIndex"`
	TypeMappings      map[string]string `json:"typeMappings" gorm:"serializer:json"`
	StatusMappings    map[string]string `json:"statusMappings" gorm:"serializer:json"`
	BugDueDateField   string            `json:"bugDueDateField" gorm:"column:bug_due_date_field"`
	TaskDueDateField  string            `json:"taskDueDateField" gorm:"column:task_due_date_field"`
	StoryDueDateField string            `json:"storyDueDateField" gorm:"column:story_due_date_field"`
}

func (teambitionScopeConfig20260727) TableName() string {
	return "_tool_teambition_scope_configs"
}

type addMissingScopeConfigColumns struct{}

func (script *addMissingScopeConfigColumns) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(basicRes, &teambitionScopeConfig20260727{})
}

func (*addMissingScopeConfigColumns) Version() uint64 {
	return 20260727000001
}

func (*addMissingScopeConfigColumns) Name() string {
	return "add missing id/created_at/updated_at columns to _tool_teambition_scope_configs"
}
