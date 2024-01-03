package actions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
)

func TestFindGithubShaHashes(t *testing.T) {
	testHtml := `
		<html>
			<head>
			</head>
			<body>
				<div></div>
				<div>
					<ul>
						<li>1</li>
						<li class="Box-row">
							<div></div>
							<p>Something</p>
							<p>Something</p>
							<p>
								Something
							</p>
							<div>

							</div>
						</li>
					</ul>
				</div>
				<div>
					<ul>
						<li>1</li>
					</ul>
				</div>
				<div></div>
			</body>
		</html>
	`

	// Test if ghList tag is correctly found
	correctLiTag := `<li class="Box-row"></li><li class="Box-row"></li><li class="Box-row"></li>
		<div>
			<li class="Box-row">
				<li class="Box-row"></li>
			</li>
		</div>
	`
	lis, err := html.Parse(strings.NewReader(correctLiTag))
	assert.NoError(t, err)
	res := []*html.Node{}
	findHtmlNodes(lis, ghTagLiFinder, &res)
	assert.Len(t, res, 5)

	// Test if incorrect tag is correctly not found
	correctLiTag = `<div class="Box-row">
		<p></p>
	</div>`
	lis, err = html.Parse(strings.NewReader(correctLiTag))
	assert.NoError(t, err)
	res = []*html.Node{}
	findHtmlNodes(lis, ghTagLiFinder, &res)
	assert.Len(t, res, 0)

	tree, err := html.Parse(strings.NewReader(testHtml))
	assert.NoError(t, err)
	assert.NotNil(t, tree)

	res2 := []*html.Node{}
	findHtmlNodes(tree, ghTagLiFinder, &res2)
	assert.Len(t, res2, 1)

}
