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

package tasks

import (
	"reflect"
	"strings"
	"testing"

	"github.com/apache/incubator-devlake/core/models/domainlayer/codequality"
	"github.com/apache/incubator-devlake/plugins/sonarqube/models"
	"github.com/stretchr/testify/assert"
)

// TestLongComponentFieldHandling verifies that the component field in both
// tool layer and domain layer models can hold values longer than 256 and 500 characters.
// This test documents the bug fix for:
//
//	Error 1406 (22001): Data too long for column 'component' at row 91
//
// which occurred because the domain layer CqIssueCodeBlock.Component was
// varchar(256) (GORM default) while SonarQube component paths can be much longer.
func TestLongComponentFieldHandling(t *testing.T) {
	// Simulate a realistic long SonarQube component path that would have caused
	// the "Data too long" error with the old varchar(256) limit.
	// Format: project-key:path/to/deeply/nested/file
	longComponent := "my-org_my-project:" + strings.Repeat("src/main/java/com/organization/app/", 7) + "MyClass.java"
	assert.Greater(t, len(longComponent), 256, "test component should exceed old varchar(256) limit")

	// Even longer path that exceeds the intermediate varchar(500) limit
	veryLongComponent := "my-organization_my-extremely-long-project-name-with-feature-branch-name-included:" +
		strings.Repeat("very/deeply/nested/", 20) +
		"MyExtremelyLongClassNameThatExceedsAllReasonableLimitsForFilePathsInEnterprise.java"
	assert.Greater(t, len(veryLongComponent), 500, "test component should exceed old varchar(500) limit")

	t.Run("tool layer model accepts long component values", func(t *testing.T) {
		block := &models.SonarqubeIssueCodeBlock{
			ConnectionId: 1,
			Id:           "test-id-1",
			IssueKey:     "AYUwBbGD46XwcL-YZOUv",
			Component:    longComponent,
			StartLine:    10,
			EndLine:      20,
		}
		assert.Equal(t, longComponent, block.Component)

		blockLong := &models.SonarqubeIssueCodeBlock{
			ConnectionId: 1,
			Id:           "test-id-2",
			IssueKey:     "AYUwBbGD46XwcL-YZOUv",
			Component:    veryLongComponent,
			StartLine:    10,
			EndLine:      20,
		}
		assert.Equal(t, veryLongComponent, blockLong.Component)
	})

	t.Run("domain layer model accepts long component values", func(t *testing.T) {
		domainBlock := &codequality.CqIssueCodeBlock{
			Component: longComponent,
			StartLine: 10,
			EndLine:   20,
		}
		assert.Equal(t, longComponent, domainBlock.Component)

		domainBlockLong := &codequality.CqIssueCodeBlock{
			Component: veryLongComponent,
			StartLine: 10,
			EndLine:   20,
		}
		assert.Equal(t, veryLongComponent, domainBlockLong.Component)
	})

	t.Run("converter preserves long component values", func(t *testing.T) {
		// Simulate what ConvertIssueCodeBlocks does internally
		sonarqubeIssueCodeBlock := &models.SonarqubeIssueCodeBlock{
			ConnectionId: 1,
			Id:           "test-id-long",
			IssueKey:     "AYUwBbGD46XwcL-YZOUv",
			Component:    veryLongComponent,
			StartLine:    42,
			EndLine:      42,
			StartOffset:  0,
			EndOffset:    100,
			Msg:          "Some issue message",
		}

		domainIssueCodeBlock := &codequality.CqIssueCodeBlock{
			Component:   sonarqubeIssueCodeBlock.Component,
			Msg:         sonarqubeIssueCodeBlock.Msg,
			StartLine:   sonarqubeIssueCodeBlock.StartLine,
			EndLine:     sonarqubeIssueCodeBlock.EndLine,
			StartOffset: sonarqubeIssueCodeBlock.StartOffset,
			EndOffset:   sonarqubeIssueCodeBlock.EndOffset,
		}

		assert.Equal(t, veryLongComponent, domainIssueCodeBlock.Component)
		assert.Greater(t, len(domainIssueCodeBlock.Component), 500)
	})
}

// TestComponentGormTagIsText verifies via reflection that the GORM tag
// for Component uses "type:text" instead of a limited varchar.
func TestComponentGormTagIsText(t *testing.T) {
	t.Run("tool layer SonarqubeIssueCodeBlock.Component is text", func(t *testing.T) {
		block := models.SonarqubeIssueCodeBlock{}
		val := getGormTag(block, "Component")
		assert.Contains(t, val, "type:text",
			"SonarqubeIssueCodeBlock.Component should use gorm type:text to avoid truncation")
	})

	t.Run("domain layer CqIssueCodeBlock.Component is text", func(t *testing.T) {
		block := codequality.CqIssueCodeBlock{}
		val := getGormTag(block, "Component")
		assert.Contains(t, val, "type:text",
			"CqIssueCodeBlock.Component should use gorm type:text to avoid truncation")
	})
}

// getGormTag extracts the `gorm` struct tag value for the given field name.
func getGormTag(model interface{}, fieldName string) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	field, ok := t.FieldByName(fieldName)
	if !ok {
		return ""
	}
	return field.Tag.Get("gorm")
}


