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

package e2e

import (
	"testing"
	"time"

	"github.com/apache/incubator-devlake/core/models/common"
	"github.com/apache/incubator-devlake/core/models/domainlayer/codequality"
	"github.com/apache/incubator-devlake/helpers/e2ehelper"
	"github.com/apache/incubator-devlake/plugins/sonarqube/impl"
	"github.com/apache/incubator-devlake/plugins/sonarqube/models"
	"github.com/apache/incubator-devlake/plugins/sonarqube/tasks"
	"github.com/stretchr/testify/assert"
)

// TestSonarqubeIssueCodeBlockLongComponent verifies that the convertIssueCodeBlocks
// subtask can handle component paths longer than 256 and 500 characters without
// producing "Data too long for column 'component'" errors.
//
// This test reproduces the original bug:
//
//	Error 1406 (22001): Data too long for column 'component' at row 91
//
// The fix changed both tool layer and domain layer `component` columns to TEXT type.
func TestSonarqubeIssueCodeBlockLongComponent(t *testing.T) {
	var sonarqube impl.Sonarqube
	dataflowTester := e2ehelper.NewDataFlowTester(t, "sonarqube", sonarqube)

	// Migrate the tables to get the current schema (with TEXT type for component)
	dataflowTester.FlushTabler(&models.SonarqubeIssue{})
	dataflowTester.FlushTabler(&models.SonarqubeIssueCodeBlock{})
	dataflowTester.FlushTabler(&codequality.CqIssueCodeBlock{})

	// Create a component path that exceeds 256 chars (old GORM default varchar limit)
	longComponent256 := "my-org_my-project-key:src/main/java/com/myorganization/myapplication/services/implementations/deeply/nested/package/structure/subpackage/another/level/of/nesting/MyVeryLongServiceImplementationClassNameThatExceedsThe256CharacterLimit.java"
	assert.Greater(t, len(longComponent256), 256)

	// Create a component path that exceeds 500 chars (previous varchar(500) limit)
	longComponent500 := "my-organization_my-extremely-long-project-name-with-feature-branch:" +
		"src/main/java/com/enterprise/application/modules/customer/services/implementations/" +
		"internal/validation/rules/complex/nested/deeply/structured/package/hierarchy/" +
		"subsystem/core/domain/entities/aggregates/valueobjects/specifications/" +
		"MyExtremelyLongAndDescriptiveClassNameForAnEnterpriseApplicationServiceImplementation" +
		"ThatFollowsAllNamingConventionsAndExceedsReasonableLimits.java"
	assert.Greater(t, len(longComponent500), 500)

	// Insert test issue (needed for the JOIN in the converter)
	issueKey := "TEST-LONG-COMPONENT-ISSUE"
	projectKey := "test-long-component-project"
	testIssue := &models.SonarqubeIssue{
		ConnectionId: 1,
		IssueKey:     issueKey,
		ProjectKey:   projectKey,
		Component:    longComponent500,
		Rule:         "java:S3776",
		Severity:     "CRITICAL",
	}
	result := dataflowTester.Db.Create(testIssue)
	assert.NoError(t, result.Error, "inserting issue with long component should succeed")

	// Insert code blocks with long component paths
	codeBlocks := []*models.SonarqubeIssueCodeBlock{
		{
			ConnectionId: 1,
			Id:           "test-long-component-block-256",
			IssueKey:     issueKey,
			Component:    longComponent256,
			StartLine:    10,
			EndLine:      20,
			StartOffset:  0,
			EndOffset:    50,
			Msg:          "Test message for component > 256 chars",
		},
		{
			ConnectionId: 1,
			Id:           "test-long-component-block-500",
			IssueKey:     issueKey,
			Component:    longComponent500,
			StartLine:    42,
			EndLine:      42,
			StartOffset:  5,
			EndOffset:    80,
			Msg:          "Test message for component > 500 chars",
		},
	}
	for _, block := range codeBlocks {
		result := dataflowTester.Db.Create(block)
		assert.NoError(t, result.Error, "inserting code block with long component should succeed (tool layer)")
	}

	// Run the converter subtask - this is where the original error occurred
	taskData := &tasks.SonarqubeTaskData{
		Options: &tasks.SonarqubeOptions{
			ConnectionId: 1,
			ProjectKey:   projectKey,
		},
		TaskStartTime: time.Now(),
	}

	// This would fail with the old schema:
	// Error 1406 (22001): Data too long for column 'component' at row N
	dataflowTester.Subtask(tasks.ConvertIssueCodeBlocksMeta, taskData)

	// Verify the converted domain layer records have the full component path
	var domainBlocks []codequality.CqIssueCodeBlock
	result = dataflowTester.Db.Find(&domainBlocks)
	assert.NoError(t, result.Error)
	assert.GreaterOrEqual(t, len(domainBlocks), 2, "should have at least 2 converted code blocks")

	// Verify the long component values were preserved without truncation
	foundLong256 := false
	foundLong500 := false
	for _, block := range domainBlocks {
		if block.Component == longComponent256 {
			foundLong256 = true
			assert.Equal(t, "Test message for component > 256 chars", block.Msg)
		}
		if block.Component == longComponent500 {
			foundLong500 = true
			assert.Equal(t, "Test message for component > 500 chars", block.Msg)
		}
	}
	assert.True(t, foundLong256, "domain layer should contain the 256+ char component without truncation")
	assert.True(t, foundLong500, "domain layer should contain the 500+ char component without truncation")

	// Also verify the tool layer stored the values correctly
	var toolBlocks []models.SonarqubeIssueCodeBlock
	result = dataflowTester.Db.Where("connection_id = ? AND issue_key = ?", 1, issueKey).Find(&toolBlocks)
	assert.NoError(t, result.Error)
	assert.Equal(t, 2, len(toolBlocks))
	for _, block := range toolBlocks {
		assert.True(t, len(block.Component) > 256,
			"tool layer component should not be truncated")
	}

	// Cleanup - ignore NoPKModel fields in verification
	_ = common.NoPKModel{}
}

