package dto

import (
	"golang-server-starter/internal/pkg/query"
	"golang-server-starter/internal/pkg/token"
)

// PageQuery 分页查询参数，内嵌 query.Page 以复用分页与排序逻辑
//
// 固定的请求契约：新增列表接口时直接内嵌它，即可获得分页参数与
// 排序字段白名单校验（见 repository.Page）。
type PageQuery struct {
	query.Page
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,username"`
	Email    string `json:"email" binding:"required,email,max=128"`
	Password string `json:"password" binding:"required,password"`
	Nickname string `json:"nickname" binding:"omitempty,max=64"`
}

// LoginRequest 登录请求，account 支持用户名或邮箱
type LoginRequest struct {
	Account  string `json:"account" binding:"required,max=128"`
	Password string `json:"password" binding:"required,max=64"`
}

// RefreshTokenRequest 刷新令牌请求
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,max=64"`
	NewPassword string `json:"new_password" binding:"required,password"`
}

// AuthResponse 登录/刷新成功响应
type AuthResponse struct {
	Token   *token.Pair `json:"token"`
	Account *Account    `json:"account"`
}

// ClientMeta 客户端元信息，用于记录登录来源
type ClientMeta struct {
	IP        string
	UserAgent string
}
