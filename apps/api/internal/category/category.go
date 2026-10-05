package category

type Definition struct {
	ID   int64
	Slug string
}

const (
	AnnouncementsID int64 = 1
	FeedbackID      int64 = 2
	NovelID         int64 = 100
)

var definitions = [...]Definition{
	{ID: NovelID, Slug: "novel"},
	{ID: AnnouncementsID, Slug: "announcements"},
	{ID: FeedbackID, Slug: "feedback"},
}

func List() []Definition {
	return append([]Definition(nil), definitions[:]...)
}

func FindByID(id int64) (Definition, bool) {
	for _, item := range definitions {
		if item.ID == id {
			return item, true
		}
	}
	return Definition{}, false
}

func FindBySlug(slug string) (Definition, bool) {
	for _, item := range definitions {
		if item.Slug == slug {
			return item, true
		}
	}
	return Definition{}, false
}
