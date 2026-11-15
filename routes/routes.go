package routes

import (
	"net/http"

	"bluebell/controller"
	_ "bluebell/docs"
	"bluebell/logger"
	"bluebell/middlewares"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"time"
)

func Setup() *gin.Engine {
	r := gin.New()
	r.Use(logger.GinLogger(), logger.GinRecovery(true), middlewares.RateLimitMiddleware(2*time.Second, 100)) // 限流中间件，2秒钟最多处理100个请求

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// 注册 Swagger 文档路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
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
	auth.GET("/me", middlewares.JWTAuthMiddleware(), controller.MeHandler)

	// 问答相关路由
	v1.POST("/questions", middlewares.JWTAuthMiddleware(), controller.CreateQuestionHandler)
	v1.GET("/questions/:id", controller.GetQuestionDetailHandler)
	v1.POST("/questions/:id/answers", middlewares.JWTAuthMiddleware(), controller.CreateAnswerHandler)
	v1.GET("/questions/:id/answers", controller.ListAnswersHandler)

	// Feed 相关路由（following 需登录，其余公开）
	v1.GET("/feed/:type", controller.GetFeedHandler)

	// 互动相关路由
	v1.POST("/users/:id/follow", middlewares.JWTAuthMiddleware(), controller.FollowUserHandler)
	v1.DELETE("/users/:id/follow", middlewares.JWTAuthMiddleware(), controller.UnfollowUserHandler)
	v1.POST("/answers/:answer_id/vote", middlewares.JWTAuthMiddleware(), controller.VoteAnswerHandler)
	v1.POST("/answers/:answer_id/collect", middlewares.JWTAuthMiddleware(), controller.CollectAnswerHandler)
	v1.DELETE("/answers/:answer_id/collect", middlewares.JWTAuthMiddleware(), controller.UncollectAnswerHandler)
	v1.GET("/answers/:answer_id/interactions", controller.GetAnswerInteractionsHandler)
	v1.POST("/questions/:id/follow", middlewares.JWTAuthMiddleware(), controller.FollowQuestionHandler)
	v1.DELETE("/questions/:id/follow", middlewares.JWTAuthMiddleware(), controller.UnfollowQuestionHandler)
	v1.GET("/questions/:id/interactions", controller.GetQuestionInteractionsHandler)

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

	pprof.Register(r) // 注册 pprof 路由

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"msg": "404 not found",
		})
	})

	return r
}
