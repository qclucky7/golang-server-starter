package graphql

import (
	"context"

	"gin-quick-start/internal/apperr"
	"gin-quick-start/internal/constant"
	"gin-quick-start/internal/graphql/gqlctx"
	"gin-quick-start/internal/model"
)

// currentAccount 取当前登录账号，未登录返回统一的鉴权错误。
//
// 为什么不在端点上做强制鉴权：GraphQL 只有一个端点，强制鉴权会让
// health 这类公开查询也必须带令牌。所以改成可选鉴权 + 需要登录的字段自己判，
// 错误码沿用 REST 时代的 auth.token.missing，客户端不需要换一套错误码表。
func currentAccount(ctx context.Context) (*model.Account, error) {
	account := gqlctx.Account(ctx)
	if account == nil {
		return nil, apperr.ErrAuthHeaderMissing.WithExtra(constant.HeaderAuthorization)
	}
	return account, nil
}
