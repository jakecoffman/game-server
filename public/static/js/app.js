var app = angular.module("app", ['ngRoute', 'ngResource', 'monospaced.qrcode'], function($routeProvider){
	$routeProvider.when("/", {
		templateUrl: "/home.html",
		controller: "HomeCtl"
	}).when("/game/:id", {
		templateUrl: "/game.html",
		controller: "GameCtl"
	});
});

app.controller("MainCtl", function($scope){
	
})

app.controller("HomeCtl", function($scope, $http, $location){
	$scope.newGame = function(){
		$http({
			method: "post",
			url: "/game"
		}).success(function(data) {
			$location.path("/game/" + data.uuid)
		}).error(function(err) {
			alert(err.message);
		})
	}
});

app.controller('GameCtl', function($scope, $http, $routeParams){
	$scope.id = $routeParams.id;
	$scope.state = "waiting";

	var conn = new WebSocket("ws://localhost:3000/ws/" + $scope.id);

	conn.onclose = function(e){
		$scope.$apply(function(){
			console.log(e);
			$scope.state = "error";
			$scope.error = e;
		});
	};

	conn.onopen = function(e){
		$scope.$apply(function(){
			console.log("CONNECTED");
			console.log(e);
			$scope.state = "ok";
		});
	};

	conn.onmessage = function(e){
		$scope.$apply(function(){
			console.log(e);
		});
	};
});
