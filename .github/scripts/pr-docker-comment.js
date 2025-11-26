// Script to post Docker image details as a comment on a PR

module.exports = async ({ github, context, core }) => {
  const prNumber = context.issue.number;
  const sha = context.sha.substring(0, 7); // Short SHA
  const prTag = `snapshot-pr-${prNumber}`;
  const shaTag = `snapshot-sha-${sha}`;
  const registryName = process.env.REGISTRY;
  const imageName = process.env.IMAGE_NAME;

  // Get current time for timestamp
  const now = new Date();
  const timestamp = now.toISOString().replace(/T/, " ").replace(/\..+/, "") +
    " UTC";

  // Format the comment
  const commentHeader = `## 🐋 Docker Images Built`;
  const comment = `${commentHeader}

Docker images for this PR have been built and pushed to Docker Hub at \`${timestamp}\`:

| Type | Tag | Pull Command | Docker Hub Link |
| ---- | --- | ------------ | --------------- |
| PR | \`${prTag}\` | \`docker pull ${registryName}/${imageName}:${prTag}\` | [View on Docker Hub](https://${registryName}/r/${imageName}/tags?name=${prTag}) |
| Commit | \`${shaTag}\` | \`docker pull ${registryName}/${imageName}:${shaTag}\` | [View on Docker Hub](https://${registryName}/r/${imageName}/tags?name=${shaTag}) |

### Running the image

\`\`\`bash
# Run the PR image
docker run --rm ${registryName}/${imageName}:${prTag} --help

# Or run the specific commit image
docker run --rm ${registryName}/${imageName}:${shaTag} --help
\`\`\`

### Image Details

- Built from commit: [\`${sha}\`](https://github.com/${context.repo.owner}/${context.repo.repo}/commit/${context.sha})
- Build job: [View workflow run](https://github.com/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId})
`;

  // Check for existing comments to avoid duplicates
  const { data: comments } = await github.rest.issues.listComments({
    issue_number: prNumber,
    owner: context.repo.owner,
    repo: context.repo.repo,
  });

  // Look for an existing Docker image comment to update
  const existingComment = comments.find((comment) =>
    comment.body.includes(commentHeader)
  );

  if (existingComment) {
    // Update existing comment
    await github.rest.issues.updateComment({
      comment_id: existingComment.id,
      owner: context.repo.owner,
      repo: context.repo.repo,
      body: comment,
    });
    core.info(`Updated existing comment ID ${existingComment.id}`);
  } else {
    // Create new comment
    await github.rest.issues.createComment({
      issue_number: prNumber,
      owner: context.repo.owner,
      repo: context.repo.repo,
      body: comment,
    });
    core.info(`Created new comment`);
  }
};
