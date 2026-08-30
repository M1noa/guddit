package output

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"github.com/minoa/guddit/internal/reddit"
)

var postCSVHeader = []string{
	"id", "title", "author", "subreddit", "score", "upvote_ratio",
	"num_comments", "created_utc", "permalink", "url", "domain",
	"is_self", "is_video", "over_18", "spoiler", "stickied", "locked",
	"archived", "flair", "gilded", "total_awards",
}

func writeCSV(result any, w io.Writer) error {
	cw := csv.NewWriter(w)
	switch v := result.(type) {
	case withPaginator:
		return writePostsCSV(cw, v.posts)
	case []reddit.Post:
		return writePostsCSV(cw, v)
	case []reddit.Comment:
		return writeCommentsCSV(cw, v)
	case *reddit.PostWithComments:
		if err := cw.Write(postCSVHeader); err != nil {
			return err
		}
		if err := writePostRow(cw, &v.Post); err != nil {
			return err
		}
		if err := writeCommentsCSVRows(cw, v.Comments, 0); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.Subreddit:
		if err := cw.Write([]string{"name", "title", "subscribers", "active", "created_utc", "over_18", "type", "description"}); err != nil {
			return err
		}
		if err := cw.Write([]string{
			v.Data.Name, v.Data.Title, strconv.Itoa(v.Data.Subscribers),
			strconv.Itoa(v.Data.ActiveUsers), ftoa(v.Data.CreatedUTC),
			strconv.FormatBool(v.Data.Over18), v.Data.SubredditType,
			v.Data.PublicDesc,
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.User:
		if err := cw.Write([]string{"name", "link_karma", "comment_karma", "total_karma", "created_utc", "is_gold", "is_mod", "public_desc"}); err != nil {
			return err
		}
		if err := cw.Write([]string{
			v.Data.Name, strconv.Itoa(v.Data.LinkKarma),
			strconv.Itoa(v.Data.CommentKarma), strconv.Itoa(v.Data.TotalKarma),
			ftoa(v.Data.CreatedUTC), strconv.FormatBool(v.Data.IsGold),
			strconv.FormatBool(v.Data.IsMod), v.Data.Subreddit.PublicDesc,
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.Me:
		if err := cw.Write([]string{"name", "link_karma", "comment_karma", "total_karma", "created_utc", "is_gold", "is_mod"}); err != nil {
			return err
		}
		if err := cw.Write([]string{
			v.Name, strconv.Itoa(v.LinkKarma), strconv.Itoa(v.CommentKarma),
			strconv.Itoa(v.TotalKarma), ftoa(v.CreatedUTC),
			strconv.FormatBool(v.IsGold), strconv.FormatBool(v.IsMod),
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case *reddit.WikiPage:
		if err := cw.Write([]string{"content_md", "revision_by", "revision_date"}); err != nil {
			return err
		}
		if err := cw.Write([]string{v.Data.ContentMD, v.Data.RevisionBy.Data.Author, ftoa(v.Data.RevisionDate)}); err != nil {
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

func writePostsCSV(cw *csv.Writer, posts []reddit.Post) error {
	if err := cw.Write(postCSVHeader); err != nil {
		return err
	}
	for i := range posts {
		if err := writePostRow(cw, &posts[i]); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writePostRow(cw *csv.Writer, p *reddit.Post) error {
	return cw.Write([]string{
		p.ID, p.Title, p.Author, p.Subreddit,
		strconv.Itoa(p.Score), strconv.FormatFloat(p.UpvoteRatio, 'f', 2, 64),
		strconv.Itoa(p.NumComments), ftoa(p.CreatedUTC),
		p.Permalink, p.URL, p.Domain,
		strconv.FormatBool(p.IsSelf), strconv.FormatBool(p.IsVideo),
		strconv.FormatBool(p.Over18), strconv.FormatBool(p.Spoiler),
		strconv.FormatBool(p.Stickied), strconv.FormatBool(p.Locked),
		strconv.FormatBool(p.Archived), p.LinkFlairText,
		strconv.Itoa(p.Gilded), strconv.Itoa(p.TotalAwards),
	})
}

var commentCSVHeader = []string{
	"id", "author", "body", "score", "depth", "created_utc",
	"subreddit", "link_id", "parent_id", "is_submitter",
	"stickied", "gilded", "permalink",
}

func writeCommentsCSV(cw *csv.Writer, comments []reddit.Comment) error {
	if err := cw.Write(commentCSVHeader); err != nil {
		return err
	}
	if err := writeCommentsCSVRows(cw, comments, 0); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func writeCommentsCSVRows(cw *csv.Writer, comments []reddit.Comment, depth int) error {
	for i := range comments {
		c := &comments[i]
		if c.IsMore() {
			if err := cw.Write([]string{
				strings.Join(c.Data.Children, ";"), "", "", "",
				strconv.Itoa(depth), "", "", "", "", "", "", "", "",
			}); err != nil {
				return err
			}
			continue
		}
		if err := cw.Write([]string{
			c.Data.ID, c.Data.Author, c.Data.Body,
			strconv.Itoa(c.Data.Score), strconv.Itoa(depth),
			ftoa(c.Data.CreatedUTC), c.Data.Subreddit,
			c.Data.LinkID, c.Data.ParentID,
			strconv.FormatBool(c.Data.IsSubmitter),
			strconv.FormatBool(c.Data.Stickied),
			strconv.Itoa(c.Data.Gilded), c.Data.Permalink,
		}); err != nil {
			return err
		}
		if err := writeCommentsCSVRows(cw, c.Replies, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
