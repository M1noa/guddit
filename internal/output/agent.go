package output

import (
	"encoding/csv"
	"io"
	"strconv"

	"github.com/minoa/guddit/internal/reddit"
)

// agent format is a token-cheap, stable subset of CSV meant for LLMs and
// scripts. Fewer columns than the full csv format; no color, no decoration.
var agentPostHeader = []string{
	"id", "subreddit", "author", "score", "num_comments", "title", "url",
}

func writeAgent(result any, w io.Writer) error {
	cw := csv.NewWriter(w)
	switch v := result.(type) {
	case withPaginator:
		return writeAgentPosts(cw, v.posts)
	case []reddit.Post:
		return writeAgentPosts(cw, v)
	case []reddit.Comment:
		return writeCommentsCSV(cw, v)
	case *reddit.PostWithComments:
		if err := cw.Write(agentPostHeader); err != nil {
			return err
		}
		if err := writeAgentPostRow(cw, &v.Post); err != nil {
			return err
		}
		if err := writeCommentsCSVRows(cw, v.Comments, 0); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.Subreddit:
		if err := cw.Write([]string{"name", "subscribers", "active", "over_18", "type", "title"}); err != nil {
			return err
		}
		if err := cw.Write([]string{
			v.Data.Name, strconv.Itoa(v.Data.Subscribers),
			strconv.Itoa(v.Data.ActiveUsers), strconv.FormatBool(v.Data.Over18),
			v.Data.SubredditType, v.Data.Title,
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.User:
		if err := cw.Write([]string{"name", "link_karma", "comment_karma", "total_karma", "is_gold", "is_mod"}); err != nil {
			return err
		}
		if err := cw.Write([]string{
			v.Data.Name, strconv.Itoa(v.Data.LinkKarma),
			strconv.Itoa(v.Data.CommentKarma), strconv.Itoa(v.Data.TotalKarma),
			strconv.FormatBool(v.Data.IsGold), strconv.FormatBool(v.Data.IsMod),
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.Me:
		if err := cw.Write([]string{"name", "link_karma", "comment_karma", "total_karma", "is_gold", "is_mod"}); err != nil {
			return err
		}
		if err := cw.Write([]string{
			v.Name, strconv.Itoa(v.LinkKarma), strconv.Itoa(v.CommentKarma),
			strconv.Itoa(v.TotalKarma), strconv.FormatBool(v.IsGold),
			strconv.FormatBool(v.IsMod),
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.WikiPage:
		if err := cw.Write([]string{"revision_by", "revision_date", "content_md"}); err != nil {
			return err
		}
		if err := cw.Write([]string{v.Data.RevisionBy.Data.Author, ftoa(v.Data.RevisionDate), v.Data.ContentMD}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.KarmaBreakdown:
		if err := cw.Write([]string{"subreddit", "karma", "comment_karma", "link_karma"}); err != nil {
			return err
		}
		for _, sr := range v.Data.Subreddits {
			if err := cw.Write([]string{sr.SR, strconv.Itoa(sr.Karma), strconv.Itoa(sr.CommentKarma), strconv.Itoa(sr.LinkKarma)}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	default:
		return writeJSON(result, w)
	}
}

func writeAgentPosts(cw *csv.Writer, posts []reddit.Post) error {
	if err := cw.Write(agentPostHeader); err != nil {
		return err
	}
	for i := range posts {
		if err := writeAgentPostRow(cw, &posts[i]); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeAgentPostRow(cw *csv.Writer, p *reddit.Post) error {
	return cw.Write([]string{
		p.ID, p.Subreddit, p.Author, strconv.Itoa(p.Score),
		strconv.Itoa(p.NumComments), p.Title, p.Permalink,
	})
}