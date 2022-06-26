package querying

import (
	"expenses-app/pkg/app"
	"expenses-app/pkg/domain/expense"
)

type GetCategoriesResp struct {
	Categories map[string]string
}

//type GetCategoriesReq struct {}

type CategoryGetter struct {
	logger   app.Logger
	expenses expense.Expenses
}

func NewCategoryGetter(l app.Logger, e expense.Expenses) *CategoryGetter {
	return &CategoryGetter{l, e}
}

func (s *CategoryGetter) Get() (*GetCategoriesResp, error) {
	categories, err := s.expenses.GetCategories()
	if err != nil {
		s.logger.Err("Could not get categories from storage")
		return nil, err
	}
	resp := GetCategoriesResp{}
	resp.Categories = make(map[string]string)
	for _, c := range categories {
		resp.Categories[string(c.ID)] = string(c.Name)
	}
	return &resp, nil
}
