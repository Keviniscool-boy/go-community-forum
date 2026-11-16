package routes

import (
	"net/http"

	"bluebell/controller"
	_ "bluebell/docs"
	"bluebell/logger"
	"bluebell/middlewares"
	"bluebell/models"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/spf13/viper"
	"time"
)

func Setup() *gin.Engine {
	r := gin.New()
	r.Use(middlewares.RequestIDMiddleware(), logger.GinLogger(), logger.GinRecovery(true), middlewares.RateLimitMiddleware(2*time.Second, 100)) // 请求追踪 + 限流

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	v1 := r.Group("api/v1")

	// 注册路由
	v1.POST("/signup", controller.SignUpHandler)
	// 登录路由
	v1.POST("/login", controller.LoginHandler)

	// 邮箱认证相关路由（新前端统一使用 /auth/*，旧 /signup /login 迁移期保留兼容）
	auth := v1.Group("/auth")
	auth.POST("/send-code", controller.SendCodeHandler)
	auth.POST("/register", controller.RegisterHandler)
	auth.POST("/login", controller.EmailLoginHandler)
	auth.POST("/password/reset/verify", controller.VerifyResetCodeHandler)
	auth.POST("/password/reset", controller.ResetWithTokenHandler)
	auth.PUT("/profile", middlewares.JWTAuthMiddleware(), controller.UpdateProfileHandler)
	auth.GET("/me", middlewares.JWTAuthMiddleware(), controller.MeHandler)
	auth.POST("/logout-all", middlewares.JWTAuthMiddleware(), controller.LogoutAllHandler)
	auth.POST("/email/bind", middlewares.JWTAuthMiddleware(), controller.BindEmailHandler)
	// 发送绑定验证码需要登录（purpose=bind_email），与公开发码路由分开注册
	auth.POST("/send-code/bind", middlewares.JWTAuthMiddleware(), controller.BindSendCodeHandler)

	// 问答相关路由
	v1.POST("/questions", middlewares.JWTAuthMiddleware(), controller.CreateQuestionHandler)
	v1.GET("/questions/:id", middlewares.OptionalAuthMiddleware(), controller.GetQuestionDetailHandler)
	v1.POST("/questions/:id/answers", middlewares.JWTAuthMiddleware(), controller.CreateAnswerHandler)
	v1.GET("/questions/:id/answers", controller.ListAnswersHandler)

	// Feed 相关路由（following 需登录，其余公开）
	v1.GET("/feed/:type", middlewares.OptionalAuthMiddleware(), controller.GetFeedHandler)

	// 互动相关路由
	v1.POST("/users/:id/follow", middlewares.JWTAuthMiddleware(), controller.FollowUserHandler)
	v1.DELETE("/users/:id/follow", middlewares.JWTAuthMiddleware(), controller.UnfollowUserHandler)
	v1.PATCH("/answers/:id", middlewares.JWTAuthMiddleware(), controller.EditAnswerHandler)
	v1.PUT("/answers/:id", middlewares.JWTAuthMiddleware(), controller.EditAnswerHandler)
	v1.DELETE("/answers/:id", middlewares.JWTAuthMiddleware(), controller.DeleteAnswerHandler)
	v1.POST("/answers/:id/vote", middlewares.JWTAuthMiddleware(), controller.VoteAnswerHandler)
	v1.POST("/answers/:id/collect", middlewares.JWTAuthMiddleware(), controller.CollectAnswerHandler)
	v1.DELETE("/answers/:id/collect", middlewares.JWTAuthMiddleware(), controller.UncollectAnswerHandler)
	v1.GET("/answers/:id/interactions", middlewares.OptionalAuthMiddleware(), controller.GetAnswerInteractionsHandler)
	v1.PATCH("/questions/:id", middlewares.JWTAuthMiddleware(), controller.EditQuestionHandler)
	v1.PUT("/questions/:id", middlewares.JWTAuthMiddleware(), controller.EditQuestionHandler)
	v1.DELETE("/questions/:id", middlewares.JWTAuthMiddleware(), controller.DeleteQuestionHandler)
	v1.POST("/questions/:id/follow", middlewares.JWTAuthMiddleware(), controller.FollowQuestionHandler)
	v1.DELETE("/questions/:id/follow", middlewares.JWTAuthMiddleware(), controller.UnfollowQuestionHandler)
	v1.GET("/questions/:id/interactions", middlewares.OptionalAuthMiddleware(), controller.GetQuestionInteractionsHandler)

	// 评论相关路由
	v1.POST("/comments", middlewares.JWTAuthMiddleware(), controller.CreateCommentHandler)
	v1.GET("/comments", controller.ListCommentsHandler)

	// 通知相关路由
	v1.GET("/notifications", middlewares.JWTAuthMiddleware(), controller.ListNotificationsHandler)
	v1.GET("/notifications/unread-count", middlewares.JWTAuthMiddleware(), controller.UnreadCountHandler)
	v1.POST("/notifications/read-all", middlewares.JWTAuthMiddleware(), controller.MarkAllNotificationsReadHandler)

	// 用户主页路由
	v1.GET("/users/:id", controller.GetUserProfileHandler)
	v1.GET("/users/:id/questions", controller.ListUserQuestionsHandler)
	v1.GET("/users/:id/answers", controller.ListUserAnswersHandler)

	// 搜索（公开，游客可搜公开内容）
	v1.GET("/search", controller.SearchHandler)
	v1.GET("/search/suggestions", controller.SearchSuggestionsHandler)

	// 文章（OptionalAuth 详情/列表公开）
	v1.POST("/articles", middlewares.JWTAuthMiddleware(), controller.CreateArticleHandler)
	v1.GET("/articles", controller.ListArticlesHandler)
	v1.GET("/articles/:id", middlewares.OptionalAuthMiddleware(), controller.GetArticleDetailHandler)
	v1.PATCH("/articles/:id", middlewares.JWTAuthMiddleware(), controller.EditArticleHandler)
	v1.DELETE("/articles/:id", middlewares.JWTAuthMiddleware(), controller.DeleteArticleHandler)

	// 专栏
	v1.POST("/columns", middlewares.JWTAuthMiddleware(), controller.CreateColumnHandler)
	v1.GET("/columns", controller.ListColumnsHandler)
	v1.GET("/columns/:id", controller.GetColumnDetailHandler)
	v1.GET("/columns/:id/articles", controller.ListColumnArticlesHandler)
	v1.PATCH("/columns/:id", middlewares.JWTAuthMiddleware(), controller.EditColumnHandler)
	v1.DELETE("/columns/:id", middlewares.JWTAuthMiddleware(), controller.DeleteColumnHandler)

	// 举报（登录用户）
	v1.POST("/reports", middlewares.JWTAuthMiddleware(), controller.CreateReportHandler)

	// 媒体与 Markdown（登录用户上传）
	v1.POST("/media/upload", middlewares.JWTAuthMiddleware(), controller.UploadMediaHandler)
	v1.GET("/media/*key", controller.MediaServeHandler)
	v1.POST("/markdown/preview", middlewares.JWTAuthMiddleware(), controller.PreviewMarkdownHandler)

	// 草稿箱（登录用户）
	drafts := v1.Group("/drafts")
	drafts.Use(middlewares.JWTAuthMiddleware())
	{
		drafts.POST("", controller.CreateDraftHandler)
		drafts.GET("", controller.ListDraftsHandler)
		drafts.GET("/:id", controller.GetDraftHandler)
		drafts.PATCH("/:id", controller.UpdateDraftHandler)
		drafts.DELETE("/:id", controller.DeleteDraftHandler)
		drafts.POST("/:id/publish", controller.PublishDraftHandler)
	}

	// 收藏夹 / 历史（登录用户）
	v1.GET("/bookmark-folders", middlewares.JWTAuthMiddleware(), controller.ListBookmarkFoldersHandler)
	v1.POST("/bookmark-folders", middlewares.JWTAuthMiddleware(), controller.CreateBookmarkFolderHandler)
	v1.DELETE("/bookmark-folders/:id", middlewares.JWTAuthMiddleware(), controller.DeleteBookmarkFolderHandler)
	v1.GET("/bookmark-folders/:id/items", middlewares.JWTAuthMiddleware(), controller.ListBookmarkItemsHandler)
	v1.POST("/bookmark-folders/:id/items", middlewares.JWTAuthMiddleware(), controller.AddBookmarkItemHandler)
	v1.DELETE("/bookmark-folders/:id/items/:type/:target_id", middlewares.JWTAuthMiddleware(), controller.RemoveBookmarkItemHandler)
	v1.GET("/history", middlewares.JWTAuthMiddleware(), controller.ListViewHistoryHandler)
	v1.DELETE("/history", middlewares.JWTAuthMiddleware(), controller.ClearHistoryHandler)
	v1.POST("/views", middlewares.JWTAuthMiddleware(), controller.RecordViewHandler)

	// 推荐事件埋点（游客与登录用户均可；登录态注入 user_id）
	v1.POST("/events", middlewares.OptionalAuthMiddleware(), controller.BatchReportEventsHandler)
	v1.GET("/recommendation-version", controller.RecommendationVersionHandler)

	// 管理后台（moderator/admin；封禁与配置仅 admin）
	adminGroup := v1.Group("/admin")
	adminGroup.Use(middlewares.JWTAuthMiddleware(), middlewares.RequireRoleMiddleware(models.RoleModerator, models.RoleAdmin))
	{
		adminGroup.GET("/reports", controller.ListReportsHandler)
		adminGroup.POST("/reports/:id/resolve", controller.ResolveReportHandler)
		adminGroup.POST("/content/:type/:id/:action", controller.ModerateContentHandler)
		adminGroup.GET("/audit-logs", controller.ListAuditLogsHandler)
		adminGroup.GET("/sensitive-words", controller.ListSensitiveWordsHandler)
		adminGroup.POST("/sensitive-words", controller.AddSensitiveWordHandler)
		adminGroup.DELETE("/sensitive-words", controller.RemoveSensitiveWordHandler)
		adminGroup.GET("/metrics/overview", controller.AdminMetricsHandler)
	}
	adminOnly := v1.Group("/admin")
	adminOnly.Use(middlewares.JWTAuthMiddleware(), middlewares.RequireRoleMiddleware(models.RoleAdmin))
	{
		adminOnly.POST("/users/:id/disable", controller.AdminSetUserStatusHandler)
	}

	// 话题相关路由
	v1.GET("/topics", controller.ListTopicsHandler)
	v1.POST("/topics", middlewares.JWTAuthMiddleware(), controller.CreateTopicHandler)
	v1.GET("/topics/:id", controller.GetTopicHandler)
	v1.POST("/topics/:id/follow", middlewares.JWTAuthMiddleware(), controller.FollowTopicHandler)
	v1.DELETE("/topics/:id/follow", middlewares.JWTAuthMiddleware(), controller.UnfollowTopicHandler)
	//jwt中间件
	v1.GET("/ping", middlewares.JWTAuthMiddleware(), func(c *gin.Context) {
		//如果是已经登录的用户，判断请求头中是否携带了token
		c.String(http.StatusOK, "PONG")
	})
	// 社区相关路由
	v1.GET("/community", controller.CommunityHandler)
	v1.GET("/community/:id", controller.CommunityDetailHandler)

	// 帖子相关路由
	v1.POST("/post", middlewares.JWTAuthMiddleware(), controller.CreatePostHandler)
	v1.GET("/post/:id", controller.GetPostDetailHandler)
	v1.GET("/posts/", controller.GetPostListHandler)

	// 投票相关路由
	v1.POST("/vote", middlewares.JWTAuthMiddleware(), controller.PostVoteController)
	// 按热度/时间排序的帖子列表
	v1.GET("/posts2/", controller.GetPostList2Handler)

	// 健康检查：存活与就绪分离，就绪检查依赖与迁移版本
	r.GET("/healthz", controller.HealthzHandler)
	r.GET("/readyz", controller.ReadyzHandler)
	r.GET("/health/live", controller.HealthzHandler)
	r.GET("/health/ready", controller.ReadyzHandler)

	// pprof 与 Swagger 仅在非生产模式注册（生产环境暴露有安全风险）
	if viper.GetString("app.mode") != "prod" {
		pprof.Register(r) // 注册 pprof 路由
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"msg": "404 not found",
		})
	})

	return r
}
